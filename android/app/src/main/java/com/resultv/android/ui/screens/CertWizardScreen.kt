package com.resultv.android.ui.screens

import android.content.ClipData
import android.content.ClipboardManager
import android.content.Context
import android.content.Intent
import android.content.res.Configuration
import android.content.res.Resources
import android.provider.Settings
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.animation.AnimatedContent
import androidx.compose.animation.core.tween
import androidx.compose.animation.fadeIn
import androidx.compose.animation.fadeOut
import androidx.compose.animation.togetherWith
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.outlined.ArrowBack
import androidx.compose.material.icons.outlined.CheckCircle
import androidx.compose.material.icons.outlined.ContentCopy
import androidx.compose.material.icons.outlined.Lock
import androidx.compose.material.icons.outlined.Shield
import androidx.compose.material.icons.outlined.VerifiedUser
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.LifecycleEventObserver
import androidx.lifecycle.compose.LocalLifecycleOwner
import com.resultv.android.R
import com.resultv.android.theme.Brand
import com.resultv.android.vpn.CertExporter
import com.resultv.android.vpn.CertInstaller
import com.resultv.android.vpn.CertStore
import com.resultv.android.vpn.CertTrustState
import com.resultv.android.vpn.SettingsRepository
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

private const val TAG = "ResultV/CertWizard"

private enum class Step { Why, Safety, DirectInstall, Save, ManualInstall, Done }

/**
 * Wizard for getting our MITM root CA trusted by the system.
 *
 * The flow forks on the OS version, because the platform's capability does:
 *
 *  - **Android 10** — `KeyChain.createInstallIntent()` still installs. One tap,
 *    one system confirmation, done. No file, no Settings trip.
 *  - **Android 11+** — apps can neither install a CA nor deep-link to the
 *    screen that does, so the best possible flow is to remove every decision
 *    around the part Android reserves for the user: the certificate goes
 *    straight to Downloads under a known name, the Settings search term is one
 *    tap to copy, and Settings is one tap to open.
 *
 * Success is never a button. [CertStore] reads the trust store on every resume
 * while an install step is showing, which works with the VPN disconnected —
 * unlike the proxy self-test that confirms interception later.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun CertWizardScreen(dataDir: String, onClose: () -> Unit) {
    val context = LocalContext.current
    val scope = rememberCoroutineScope()
    var step by rememberSaveable { mutableStateOf(Step.Why) }
    val snackbarHost = remember { SnackbarHostState() }

    InstallWatcher(
        enabled = step == Step.DirectInstall || step == Step.ManualInstall,
        dataDir = dataDir,
    ) {
        SettingsRepository.setCertTrustState(CertTrustState.TRUSTED)
        SettingsRepository.setBrowserAdBlock(true)
        step = Step.Done
    }

    Scaffold(
        containerColor = Brand.Bg,
        snackbarHost = { SnackbarHost(snackbarHost) },
        topBar = {
            TopAppBar(
                title = {},
                navigationIcon = {
                    IconButton(onClick = onClose) {
                        Icon(
                            Icons.AutoMirrored.Outlined.ArrowBack,
                            contentDescription = stringResource(R.string.cert_wizard_close),
                        )
                    }
                },
                colors = TopAppBarDefaults.topAppBarColors(containerColor = Brand.Bg),
            )
        },
    ) { padding ->
        AnimatedContent(
            targetState = step,
            transitionSpec = { fadeIn(tween(220)) togetherWith fadeOut(tween(160)) },
            label = "cert-wizard-step",
            modifier = Modifier.padding(padding),
        ) { current ->
            when (current) {
                Step.Why -> IntroStep(
                    icon = Icons.Outlined.Shield,
                    title = stringResource(R.string.cert_wizard_why_title),
                    body = stringResource(R.string.cert_wizard_why_body),
                    cta = stringResource(R.string.cert_wizard_why_cta),
                    onCta = { step = Step.Safety },
                )
                Step.Safety -> IntroStep(
                    icon = Icons.Outlined.Lock,
                    title = stringResource(R.string.cert_wizard_safety_title),
                    body = stringResource(R.string.cert_wizard_safety_body),
                    cta = stringResource(R.string.cert_wizard_safety_cta),
                    onCta = {
                        step = if (CertInstaller.canInstallDirectly()) {
                            Step.DirectInstall
                        } else {
                            Step.Save
                        }
                    },
                )
                Step.DirectInstall -> DirectInstallStep(dataDir, snackbarHost)
                Step.Save -> {
                    val saveError = stringResource(R.string.cert_wizard_save_error)
                    IntroStep(
                        icon = Icons.Outlined.VerifiedUser,
                        title = stringResource(R.string.cert_wizard_save_title),
                        body = stringResource(R.string.cert_wizard_save_body),
                        cta = stringResource(R.string.cert_wizard_save_cta),
                        onCta = {
                            scope.launch {
                                val saved = withContext(Dispatchers.IO) {
                                    runCatching { CertExporter.saveToDownloads(context, dataDir) }
                                }
                                saved.fold(
                                    onSuccess = { step = Step.ManualInstall },
                                    onFailure = {
                                        android.util.Log.w(TAG, "Saving CA to Downloads failed", it)
                                        snackbarHost.showSnackbar(saveError)
                                    },
                                )
                            }
                        },
                    )
                }
                Step.ManualInstall -> ManualInstallStep(snackbarHost)
                Step.Done -> IntroStep(
                    icon = Icons.Outlined.CheckCircle,
                    title = stringResource(R.string.cert_wizard_done_title),
                    body = stringResource(R.string.cert_wizard_done_body),
                    cta = stringResource(R.string.cert_wizard_done_cta),
                    onCta = onClose,
                )
            }
        }
    }
}

/** Android 10 path: hand the certificate to the system and let it install. */
@Composable
private fun DirectInstallStep(dataDir: String, snackbarHost: SnackbarHostState) {
    val context = LocalContext.current
    val scope = rememberCoroutineScope()
    val certError = stringResource(R.string.settings_browser_adblock_cert_error)
    // KeyChain reports nothing back about what the user chose — the trust-store
    // check on resume (InstallWatcher) is what actually decides.
    val launcher = rememberLauncherForActivityResult(
        ActivityResultContracts.StartActivityForResult(),
    ) {}

    IntroStep(
        icon = Icons.Outlined.VerifiedUser,
        title = stringResource(R.string.cert_wizard_direct_title),
        body = stringResource(R.string.cert_wizard_direct_body),
        cta = stringResource(R.string.cert_wizard_direct_cta),
        onCta = {
            try {
                launcher.launch(CertInstaller.buildInstallIntent(context, dataDir))
            } catch (t: Throwable) {
                android.util.Log.w(TAG, "Couldn't launch the system install intent", t)
                scope.launch { snackbarHost.showSnackbar(certError) }
            }
        },
    )
}

/**
 * Re-checks the trust store on every resume while [enabled], firing [onFound]
 * once. Resume is the only signal available: the user leaves for a system
 * screen and comes back, and Android tells us nothing about what happened
 * there.
 */
@Composable
private fun InstallWatcher(enabled: Boolean, dataDir: String, onFound: () -> Unit) {
    val lifecycleOwner = LocalLifecycleOwner.current
    val scope = rememberCoroutineScope()
    val currentOnFound by rememberUpdatedState(onFound)

    DisposableEffect(lifecycleOwner, enabled, dataDir) {
        if (!enabled) return@DisposableEffect onDispose {}
        val observer = LifecycleEventObserver { _, event ->
            if (event == Lifecycle.Event.ON_RESUME) {
                scope.launch {
                    if (withContext(Dispatchers.IO) { CertStore.isInstalled(dataDir) }) {
                        currentOnFound()
                    }
                }
            }
        }
        lifecycleOwner.lifecycle.addObserver(observer)
        onDispose { lifecycleOwner.lifecycle.removeObserver(observer) }
    }
}

/** Illustration-led step: icon, headline, body, one full-width action. */
@Composable
private fun IntroStep(
    icon: ImageVector,
    title: String,
    body: String,
    cta: String,
    onCta: () -> Unit,
) {
    Column(
        modifier = Modifier
            .fillMaxSize()
            .padding(horizontal = 24.dp)
            .padding(bottom = 32.dp),
    ) {
        Column(
            modifier = Modifier
                .weight(1f)
                .verticalScroll(rememberScrollState()),
            horizontalAlignment = Alignment.CenterHorizontally,
        ) {
            Spacer(Modifier.height(24.dp))
            Box(
                modifier = Modifier
                    .size(96.dp)
                    .clip(RoundedCornerShape(28.dp))
                    .background(Brand.Green.copy(alpha = 0.16f)),
                contentAlignment = Alignment.Center,
            ) {
                Icon(
                    icon,
                    contentDescription = null,
                    tint = Brand.GreenLight,
                    modifier = Modifier.size(48.dp),
                )
            }
            Spacer(Modifier.height(32.dp))
            Text(
                title,
                style = MaterialTheme.typography.headlineSmall,
                fontWeight = FontWeight.Bold,
                textAlign = TextAlign.Center,
            )
            Spacer(Modifier.height(16.dp))
            Text(
                body,
                style = MaterialTheme.typography.bodyLarge,
                color = Brand.SecondaryText,
                textAlign = TextAlign.Center,
            )
            Spacer(Modifier.height(24.dp))
        }
        Button(
            onClick = onCta,
            modifier = Modifier.fillMaxWidth().height(52.dp),
            shape = RoundedCornerShape(14.dp),
            colors = ButtonDefaults.buttonColors(containerColor = Brand.Green),
        ) {
            Text(cta, style = MaterialTheme.typography.titleMedium, fontWeight = FontWeight.Medium)
        }
    }
}

/** Android 11+ path: the user has to drive Settings, so make that as short as possible. */
@Composable
private fun ManualInstallStep(snackbarHost: SnackbarHostState) {
    val context = LocalContext.current
    val scope = rememberCoroutineScope()
    val searchTerm = rememberSettingsSearchTerm()
    val copied = stringResource(R.string.cert_wizard_install_copied)
    val settingsError = stringResource(R.string.cert_wizard_install_settings_error)

    Column(
        modifier = Modifier
            .fillMaxSize()
            .padding(horizontal = 24.dp)
            .padding(bottom = 32.dp),
    ) {
        Column(
            modifier = Modifier
                .weight(1f)
                .verticalScroll(rememberScrollState()),
            verticalArrangement = Arrangement.spacedBy(20.dp),
        ) {
            Text(
                stringResource(R.string.cert_wizard_install_title),
                style = MaterialTheme.typography.headlineSmall,
                fontWeight = FontWeight.Bold,
            )
            NumberedStep(1, stringResource(R.string.cert_wizard_install_step1))
            Row(
                modifier = Modifier
                    .fillMaxWidth()
                    .clip(RoundedCornerShape(12.dp))
                    .background(Brand.Surface)
                    .padding(start = 16.dp, end = 4.dp),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                Text(
                    searchTerm,
                    style = MaterialTheme.typography.bodyLarge,
                    modifier = Modifier.weight(1f),
                )
                IconButton(onClick = {
                    val clipboard = context.getSystemService(Context.CLIPBOARD_SERVICE) as ClipboardManager
                    clipboard.setPrimaryClip(ClipData.newPlainText(searchTerm, searchTerm))
                    // Android 13+ shows its own copy confirmation; a second one
                    // would just stack on top of it.
                    if (android.os.Build.VERSION.SDK_INT < android.os.Build.VERSION_CODES.TIRAMISU) {
                        scope.launch { snackbarHost.showSnackbar(copied) }
                    }
                }) {
                    Icon(
                        Icons.Outlined.ContentCopy,
                        contentDescription = copied,
                        tint = Brand.GreenLight,
                    )
                }
            }
            NumberedStep(2, stringResource(R.string.cert_wizard_install_step2))
            NumberedStep(3, stringResource(R.string.cert_wizard_install_step3, searchTerm))
            NumberedStep(4, stringResource(R.string.cert_wizard_install_step4))
            NumberedStep(5, stringResource(R.string.cert_wizard_install_step5))
        }
        Button(
            onClick = {
                try {
                    // Top-level Settings, not ACTION_SECURITY_SETTINGS: the
                    // search field the instructions depend on lives on the
                    // root page, and the security page doesn't have one.
                    context.startActivity(
                        Intent(Settings.ACTION_SETTINGS).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK),
                    )
                } catch (t: Throwable) {
                    android.util.Log.w(TAG, "Couldn't open settings", t)
                    scope.launch { snackbarHost.showSnackbar(settingsError) }
                }
            },
            modifier = Modifier.fillMaxWidth().height(52.dp),
            shape = RoundedCornerShape(14.dp),
            colors = ButtonDefaults.buttonColors(containerColor = Brand.Green),
        ) {
            Text(
                stringResource(R.string.cert_wizard_install_cta),
                style = MaterialTheme.typography.titleMedium,
                fontWeight = FontWeight.Medium,
            )
        }
    }
}

@Composable
private fun NumberedStep(number: Int, text: String) {
    Row(horizontalArrangement = Arrangement.spacedBy(12.dp)) {
        Text(
            "$number.",
            style = MaterialTheme.typography.bodyLarge,
            color = Brand.MutedText,
        )
        Text(text, style = MaterialTheme.typography.bodyLarge)
    }
}

/**
 * The label the user must actually search for, read out of the device's own
 * Settings app rather than guessed.
 *
 * OEMs rename this freely — a stock phone says "CA certificate" where MIUI
 * says something of its own — so a hardcoded per-manufacturer table would be
 * wrong on every skin we hadn't seen. Asking the Settings package for its
 * `ca_certificate` string returns whatever *that* build calls it, already in
 * the system language, which is also the right language: the term gets pasted
 * into Settings' own search, so an app running in Russian on an English phone
 * must still hand over "CA certificate".
 *
 * Falls back to our own string in the system locale when Settings has no such
 * resource (a heavily forked skin, or a renamed resource).
 */
@Composable
private fun rememberSettingsSearchTerm(): String {
    val context = LocalContext.current
    return remember(context) {
        settingsAppString(context, "ca_certificate") ?: ownStringInSystemLocale(context)
    }
}

private fun settingsAppString(context: Context, name: String): String? = runCatching {
    val res = context.packageManager.getResourcesForApplication("com.android.settings")
    val id = res.getIdentifier(name, "string", "com.android.settings")
    if (id == 0) null else res.getString(id).takeIf { it.isNotBlank() }
}.getOrNull()

private fun ownStringInSystemLocale(context: Context): String {
    val systemLocale = Resources.getSystem().configuration.locales[0]
    val config = Configuration(context.resources.configuration).apply { setLocale(systemLocale) }
    return context.createConfigurationContext(config)
        .getString(R.string.cert_wizard_install_search_term)
}
