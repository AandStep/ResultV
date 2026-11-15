package com.resultv.android.vpn

import android.content.Context
import android.util.Log
import com.resultv.android.R
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import java.io.File
import org.json.JSONObject

private const val TAG = "ResultV/AppRouting"
private const val FILE_NAME = "app_routing.json"

data class AppModeRules(val smart: AppRulesState = AppRulesState(), val global: AppRulesState = AppRulesState()) {
    fun forMode(mode: RoutingMode) = if (mode == RoutingMode.Smart) smart else global

    fun edit(mode: RoutingMode, change: (AppRulesState) -> AppRulesState): AppModeRules =
        if (mode == RoutingMode.Smart) copy(smart = change(smart)) else copy(global = change(global))
}

internal fun decodeAppModeRules(text: String): AppModeRules {
    val root = JSONObject(text)
    if (root.has("smart") || root.has("global")) {
        return AppModeRules(
            decodeAppRules(root.getJSONObject("smart").toString()),
            decodeAppRules(root.getJSONObject("global").toString()),
        )
    }
    val legacy = decodeAppRules(text)
    // Android already used the legacy direct list to exclude apps in Smart.
    return AppModeRules(
        legacy.copy(intoVpn = legacy.intoVpn - legacy.outOfVpn),
        legacy.copy(intoVpn = emptySet(), migratedFromAllowList = false),
    )
}

internal fun encodeAppModeRules(state: AppModeRules): String = JSONObject()
    .put("smart", JSONObject(encodeAppRules(state.smart)))
    .put("global", JSONObject(encodeAppRules(state.global)))
    .toString()

object AppRoutingRepository {
    private val _state = MutableStateFlow(AppModeRules())
    val state: StateFlow<AppModeRules> = _state.asStateFlow()

    @Volatile private var file: File? = null

    /**
     * Blocking or force-tunnelling our own package would cut the engine off
     * from its own proxy dial. The app catalogue in RulesScreen already hides
     * us, so this is a belt-and-braces guard for migrated/hand-edited files.
     */
    @Volatile private var ownPackage: String = ""

    @Synchronized
    fun init(ctx: Context) {
        if (file != null) return
        val f = File(ctx.filesDir, FILE_NAME)
        file = f
        ownPackage = ctx.packageName
        val loaded = load(f)
        if (loaded.smart.migratedFromAllowList) {
            AppLog.warning(R.string.log_app_rules_allowlist_migrated)
        }
        _state.value = loaded.copy(smart = loaded.smart.copy(migratedFromAllowList = false))
    }

    @Synchronized
    fun setAction(mode: RoutingMode, pkg: String, action: RuleAction) {
        if (pkg == ownPackage) return
        mutate { state ->
            state.edit(mode) { rules -> rules.without(pkg).withAction(pkg, action) }
        }
    }

    @Synchronized
    fun clearAction(mode: RoutingMode, pkg: String, action: RuleAction) =
        mutate { state -> state.edit(mode) { it.withoutAction(pkg, action) } }

    @Synchronized
    fun clearList(mode: RoutingMode, action: RuleAction) = mutate { state ->
        state.edit(mode) {
            when (action) {
                RuleAction.IntoVpn -> it.copy(intoVpn = emptySet())
                RuleAction.OutOfVpn -> it.copy(outOfVpn = emptySet())
                RuleAction.Block -> it.copy(blocked = emptySet())
            }
        }
    }

    /** Packages handed to VpnService.Builder.addAllowedApplication in Smart. */
    fun smartAllowlist(matched: Set<String>, browsers: Set<String>): Set<String> {
        val rules = _state.value.smart
        return AppTunnelMembership.smartAllowlist(
            matched = matched,
            browsers = browsers,
            intoVpn = rules.intoVpn + rules.blocked,
            outOfVpn = rules.outOfVpn - rules.blocked,
            ownPackage = ownPackage,
        )
    }

    fun engineIntoVpn(mode: RoutingMode): Set<String> = _state.value.forMode(mode).intoVpn - ownPackage

    fun engineBlocked(mode: RoutingMode): Set<String> = _state.value.forMode(mode).blocked - ownPackage

    fun disallowedPackages(mode: RoutingMode): Set<String> {
        val rules = _state.value.forMode(mode)
        return rules.outOfVpn - rules.blocked - ownPackage
    }

    private fun mutate(block: (AppModeRules) -> AppModeRules) {
        val next = block(_state.value)
        if (next == _state.value) return
        _state.value = next
        file?.let { save(it, next) }
    }

    private fun load(f: File): AppModeRules {
        if (!f.exists()) return AppModeRules()
        return try {
            decodeAppModeRules(f.readText())
        } catch (t: Throwable) {
            Log.w(TAG, "failed to read $f, starting empty", t)
            AppLog.warning(R.string.log_read_failed, f.name,
                source = AppLog.resolve(R.string.log_source_config))
            AppModeRules()
        }
    }

    private fun save(f: File, s: AppModeRules) {
        try {
            replaceRepositoryJson(f, encodeAppModeRules(s), ::decodeAppModeRules)
        } catch (t: Throwable) {
            Log.e(TAG, "failed to persist app routing", t)
            AppLog.error(R.string.log_persist_failed, f.name,
                source = AppLog.resolve(R.string.log_source_config))
        }
    }
}
