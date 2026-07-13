package com.resultv.android.vpn

import android.content.Context
import android.util.Log
import com.resultv.android.R
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import org.json.JSONArray
import org.json.JSONObject
import java.io.File

private const val TAG = "ResultV/AppRouting"
private const val FILE_NAME = "app_routing.json"

enum class AppRoutingMode { All, AllowList, DisallowList }

/**
 * Global per-app routing settings. The `Allowed` / `Disallowed` lists feed
 * directly into VpnService.Builder.add{Allowed,Disallowed}Application; the
 * two modes are mutually exclusive at the OS level, so we model that as a
 * tagged union.
 *
 * `selectedPackages` carries the active list for whichever mode is set.
 */
data class AppRoutingState(
    val mode: AppRoutingMode = AppRoutingMode.All,
    val selectedPackages: Set<String> = emptySet(),
)

object AppRoutingRepository {
    private val _state = MutableStateFlow(AppRoutingState())
    val state: StateFlow<AppRoutingState> = _state.asStateFlow()

    @Volatile private var file: File? = null

    @Synchronized
    fun init(ctx: Context) {
        if (file != null) return
        val f = File(ctx.filesDir, FILE_NAME)
        file = f
        _state.value = load(f)
    }

    @Synchronized
    fun setMode(mode: AppRoutingMode) = mutate { it.copy(mode = mode) }

    @Synchronized
    fun setSelected(packages: Set<String>) = mutate { it.copy(selectedPackages = packages) }

    @Synchronized
    fun toggle(pkg: String) = mutate {
        val next = if (pkg in it.selectedPackages) it.selectedPackages - pkg
        else it.selectedPackages + pkg
        it.copy(selectedPackages = next)
    }

    @Synchronized
    fun clearSelection() = mutate { it.copy(selectedPackages = emptySet()) }

    private fun mutate(block: (AppRoutingState) -> AppRoutingState) {
        val next = block(_state.value)
        if (next == _state.value) return
        _state.value = next
        file?.let { save(it, next) }
    }

    private fun load(f: File): AppRoutingState {
        if (!f.exists()) return AppRoutingState()
        return try {
            val root = JSONObject(f.readText())
            val mode = AppRoutingMode.entries.firstOrNull { it.name == root.optString("mode") }
                ?: AppRoutingMode.All
            val arr = root.optJSONArray("packages") ?: JSONArray()
            val pkgs = (0 until arr.length()).map { arr.getString(it) }.toSet()
            AppRoutingState(mode = mode, selectedPackages = pkgs)
        } catch (t: Throwable) {
            Log.w(TAG, "failed to read $f, starting empty", t)
            AppLog.warning(R.string.log_read_failed, f.name,
                source = AppLog.resolve(R.string.log_source_config))
            AppRoutingState()
        }
    }

    private fun save(f: File, s: AppRoutingState) {
        try {
            val arr = JSONArray()
            s.selectedPackages.sorted().forEach { arr.put(it) }
            val root = JSONObject()
                .put("mode", s.mode.name)
                .put("packages", arr)
            f.writeText(root.toString())
        } catch (t: Throwable) {
            Log.e(TAG, "failed to persist app routing", t)
            AppLog.error(R.string.log_persist_failed, f.name,
                source = AppLog.resolve(R.string.log_source_config))
        }
    }
}
