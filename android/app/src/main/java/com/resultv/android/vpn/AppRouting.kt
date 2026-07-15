package com.resultv.android.vpn

import android.content.Context
import android.util.Log
import com.resultv.android.R
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import java.io.File

private const val TAG = "ResultV/AppRouting"
private const val FILE_NAME = "app_routing.json"

/**
 * Persistence + StateFlow around [AppRulesState]. All rule logic lives in
 * AppRules.kt (pure, JVM-testable); this object owns only the file, the
 * AppLog side-effects and the own-package guard.
 */
object AppRoutingRepository {
    private val _state = MutableStateFlow(AppRulesState())
    val state: StateFlow<AppRulesState> = _state.asStateFlow()

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
        if (loaded.migratedFromAllowList) {
            AppLog.warning(R.string.log_app_rules_allowlist_migrated)
        }
        _state.value = loaded.copy(migratedFromAllowList = false)
    }

    @Synchronized
    fun setAction(pkg: String, action: RuleAction) {
        if (pkg == ownPackage) return
        mutate { it.withAction(pkg, action) }
    }

    @Synchronized
    fun clearAction(pkg: String, action: RuleAction) = mutate { it.withoutAction(pkg, action) }

    /** "Clear" button: empties only the tab the user is looking at. */
    @Synchronized
    fun clearList(action: RuleAction) = mutate { s ->
        s.listOf(action).fold(s) { acc, pkg -> acc.withoutAction(pkg, action) }
    }

    private fun AppRulesState.listOf(action: RuleAction): Set<String> = when (action) {
        RuleAction.OutOfVpn -> outOfVpn
        RuleAction.IntoVpn -> intoVpn
        RuleAction.Block -> blocked
    }

    /** Packages handed to sing-box as `package_name` → proxy (Smart only). */
    fun engineIntoVpn(): Set<String> = _state.value.intoVpn - ownPackage

    /** Packages handed to sing-box as `package_name` → reject (both modes). */
    fun engineBlocked(): Set<String> = _state.value.blocked - ownPackage

    /**
     * Packages excluded from the TUN at the OS level. Global only: in Smart the
     * "out of VPN" list is not active. Blocked apps are deliberately NOT here —
     * they must ENTER the tunnel for the reject rule to have anything to cut.
     */
    fun disallowedPackages(mode: RoutingMode): Set<String> =
        if (mode == RoutingMode.Global) _state.value.outOfVpn - ownPackage else emptySet()

    private fun mutate(block: (AppRulesState) -> AppRulesState) {
        val next = block(_state.value)
        if (next == _state.value) return
        _state.value = next
        file?.let { save(it, next) }
    }

    private fun load(f: File): AppRulesState {
        if (!f.exists()) return AppRulesState()
        return try {
            decodeAppRules(f.readText())
        } catch (t: Throwable) {
            Log.w(TAG, "failed to read $f, starting empty", t)
            AppLog.warning(R.string.log_read_failed, f.name,
                source = AppLog.resolve(R.string.log_source_config))
            AppRulesState()
        }
    }

    private fun save(f: File, s: AppRulesState) {
        try {
            f.writeText(encodeAppRules(s))
        } catch (t: Throwable) {
            Log.e(TAG, "failed to persist app routing", t)
            AppLog.error(R.string.log_persist_failed, f.name,
                source = AppLog.resolve(R.string.log_source_config))
        }
    }
}
