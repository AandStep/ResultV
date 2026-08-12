package com.resultv.android.vpn

import android.util.Log
import mobile.Mobile
import org.json.JSONObject

private const val TAG = "ResultV/Auto"

/**
 * Which member of an AUTO group we are currently connected through, and which
 * ones to try next if it fails.
 *
 * The ranking itself lives in Go (`Mobile.resolveAutoCandidates`): it probes
 * every member, re-probes the shortlist through a real TLS handshake, and
 * scores on latency, jitter and recorded failure history. This object only
 * remembers the answer for the profile in use, hands out the next candidate on
 * a failed connect, and reports outcomes back so the ranking learns.
 *
 * Deliberately NOT a StateFlow: nothing recomposes on it, and the connect path
 * reads it from a worker thread.
 */
object AutoSelection {

    data class Candidate(
        val key: String,
        val name: String,
        val rttMs: Int,
        /**
         * False when rttMs is not a real measurement — Go's "reachable but
         * could not time it" sentinel, which used to reach the UI as a
         * misleading "-1 ms". Callers must check this before formatting rttMs.
         */
        val rttKnown: Boolean,
        /** Marshaled ProxyEntry, fed back to the Go config builder verbatim. */
        val entryJson: String,
    )

    /**
     * How many members a single connect may walk through before giving up.
     * Matches AutoMaxCandidates in internal/proxy/autoselect.go — Go never
     * returns more than this, and a bound here keeps a group of dead nodes from
     * turning one connect into a long retry storm.
     */
    private const val MAX_ATTEMPTS = 5

    private var profileId: String = ""
    private var ranked: List<Candidate> = emptyList()
    private var index: Int = 0

    /** The member to connect through, or null when nothing has been resolved. */
    @Synchronized
    fun current(): Candidate? = ranked.getOrNull(index)

    /** As [current], but only if the resolution belongs to [id]. */
    @Synchronized
    fun currentFor(id: String): Candidate? = if (id == profileId) current() else null

    /**
     * Move to the next-best member after a failed connect. Returns null when the
     * list is exhausted, which the caller treats as a real connect failure.
     */
    @Synchronized
    fun advance(): Candidate? {
        if (index + 1 >= minOf(ranked.size, MAX_ATTEMPTS)) return null
        index++
        return current()
    }

    @Synchronized
    fun reset() {
        profileId = ""
        ranked = emptyList()
        index = 0
    }

    /**
     * Probe [profile]'s members and cache the ranking. Blocking — call from the
     * connect worker, never the main thread.
     *
     * A resolve that comes back empty is not an error: the sweep may have been
     * cut short, or every member may be behind a probe-hostile path that still
     * carries traffic. The caller falls back to the group head.
     */
    fun resolve(profile: Profile, dataDir: String, timeoutMs: Int = 8_000): Candidate? {
        if (!profile.isAuto || profile.entryJson.isBlank()) return null
        val raw = try {
            Mobile.resolveAutoCandidates(profile.entryJson, dataDir, timeoutMs.toLong())
        } catch (t: Throwable) {
            Log.w(TAG, "resolve failed for ${profile.name}: ${t.message}")
            return null
        }
        val list = parseCandidates(raw)
        synchronized(this) {
            profileId = profile.id
            ranked = list
            index = 0
            return current()
        }
    }

    /** Report what a real connect attempt did, so the ranking learns from it. */
    fun recordOutcome(ok: Boolean, reason: String = "") {
        val key = current()?.key ?: return
        try {
            Mobile.recordAutoConnectOutcome(key, ok, reason)
        } catch (t: Throwable) {
            Log.w(TAG, "recording outcome failed: ${t.message}")
        }
    }

    /**
     * Parse the Go payload. Any malformation yields an empty list: a broken
     * ranking must degrade to "connect to the group head", never to a crash on
     * the connect path.
     */
    internal fun parseCandidates(json: String): List<Candidate> = try {
        val arr = JSONObject(json).optJSONArray("candidates")
        buildList {
            for (i in 0 until (arr?.length() ?: 0)) {
                val o = arr?.optJSONObject(i) ?: continue
                val key = o.optString("key")
                val entry = o.optJSONObject("entry") ?: continue
                if (key.isBlank()) continue
                add(
                    Candidate(
                        key = key,
                        name = o.optString("name"),
                        rttMs = o.optInt("rttMs"),
                        rttKnown = o.optBoolean("rttKnown"),
                        entryJson = entry.toString(),
                    )
                )
            }
        }
    } catch (t: Throwable) {
        emptyList()
    }

    /** Test seam: install a ranking without going through the JNI bindings. */
    @Synchronized
    internal fun installForTest(id: String, candidates: List<Candidate>) {
        profileId = id
        ranked = candidates
        index = 0
    }
}
