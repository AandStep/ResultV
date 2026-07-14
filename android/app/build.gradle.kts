import java.util.Properties

plugins {
    id("com.android.application")
    id("org.jetbrains.kotlin.android")
    id("org.jetbrains.kotlin.plugin.compose")
}

// Release signing — keystore.properties (gitignored) carries:
//   storeFile=../release.keystore (relative to project root)
//   storePassword=...
//   keyAlias=...
//   keyPassword=...
// When the file is absent (CI without secrets, fresh clone) the release
// build falls back to debug signing so `./gradlew assembleRelease` never
// fails outright — the APK just won't be production-signed.
val keystoreProps = Properties().apply {
    val f = rootProject.file("keystore.properties")
    if (f.exists()) f.inputStream().use { load(it) }
}
val hasReleaseKeystore = keystoreProps.containsKey("storeFile")

android {
    namespace = "com.resultv.android"
    compileSdk = 34

    defaultConfig {
        applicationId = "com.resultv.android"
        minSdk = 26
        targetSdk = 34
        // Version is injected from the android-v* release tag via
        // -PversionName / -PversionCode (see release-android.yml). The
        // fallbacks keep local/dev builds working without any -P flags.
        // versionCode scheme mirrors the tag: major*10000 + minor*100 + patch
        // (so 1.0.0 == 10000, matching the historical default).
        versionName = (project.findProperty("versionName") as String?) ?: "1.0.0"
        versionCode = (project.findProperty("versionCode") as String?)?.toInt() ?: 10000
    }

    if (hasReleaseKeystore) {
        signingConfigs {
            create("release") {
                storeFile = file(keystoreProps.getProperty("storeFile"))
                storePassword = keystoreProps.getProperty("storePassword")
                keyAlias = keystoreProps.getProperty("keyAlias")
                keyPassword = keystoreProps.getProperty("keyPassword")
            }
        }
    }

    buildTypes {
        release {
            isMinifyEnabled = true
            isShrinkResources = true
            proguardFiles(
                getDefaultProguardFile("proguard-android-optimize.txt"),
                "proguard-rules.pro"
            )
            // With a real keystore, sign for production. Without one
            // (fresh clone / CI without secrets) fall back to debug signing
            // so assembleRelease still emits an *installable* APK — it just
            // won't be production-signed. Leaving signingConfig unset here
            // produces "-unsigned.apk" that adb refuses to install.
            signingConfig = if (hasReleaseKeystore) {
                signingConfigs.getByName("release")
            } else {
                signingConfigs.getByName("debug")
            }
            // Release APKs target real devices only — arm64-v8a covers
            // 99%+ of active Android devices; armeabi-v7a is the fallback
            // for older 32-bit hardware. x86/x86_64 are emulator-only.
            ndk { abiFilters += listOf("arm64-v8a", "armeabi-v7a") }
        }
        debug {
            isMinifyEnabled = false
            // Debug picks ABI from -Pdebug.abi (defaults to x86_64 for the
            // emulator). Pass -Pdebug.abi=arm64-v8a when installing on a
            // real phone over USB. Single-ABI keeps the APK ~70 MB.
            val abi = (project.findProperty("debug.abi") as String?) ?: "x86_64"
            ndk { abiFilters += abi }
        }
    }

    // Per-ABI APK splits for release — produces separate arm64-v8a and
    // armeabi-v7a APKs (~70 MB each) instead of one universal (~140 MB).
    // versionCode is offset by ABI so Play Store knows which APK to serve.
    //
    // Debug builds opt out: this list never includes x86_64, so it was
    // silently overriding the debug buildType's -Pdebug.abi selection
    // (see the `debug` block above) and forcing every debug APK onto
    // arm64-v8a/armeabi-v7a regardless of the flag. build-android.sh always
    // passes -Pdebug.abi for debug builds, so that property's presence is
    // what distinguishes "debug run" from "release run" here.
    splits {
        abi {
            isEnable = !project.hasProperty("debug.abi")
            reset()
            include("arm64-v8a", "armeabi-v7a")
            isUniversalApk = true  // fallback for sideloading
        }
    }

    // Assign distinct versionCodes per ABI so Play Store serves the right
    // APK. arm64-v8a gets the highest code (most devices prefer it).
    val abiVersionCodes = mapOf(
        "armeabi-v7a" to 1,
        "arm64-v8a" to 2,
    )
    applicationVariants.configureEach {
        outputs.configureEach {
            val output = this as? com.android.build.gradle.internal.api.ApkVariantOutputImpl ?: return@configureEach
            val abiName = output.getFilter(com.android.build.OutputFile.ABI)
            val abiCode = abiVersionCodes[abiName] ?: 0
            output.versionCodeOverride = (defaultConfig.versionCode ?: 0) + abiCode
        }
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }

    kotlinOptions {
        jvmTarget = "17"
    }

    buildFeatures {
        compose = true
        buildConfig = true
    }

    packaging {
        resources {
            excludes += "/META-INF/{AL2.0,LGPL2.1}"
        }
    }
}

dependencies {
    implementation(files("$rootDir/libs/libbox.aar"))

    implementation("androidx.core:core-ktx:1.13.1")
    implementation("androidx.lifecycle:lifecycle-runtime-ktx:2.8.7")
    implementation("androidx.lifecycle:lifecycle-runtime-compose:2.8.7")
    implementation("androidx.activity:activity-compose:1.9.3")
    // play-services-code-scanner drags in androidx.fragment:fragment:1.0.0
    // transitively, which trips the InvalidFragmentVersionForActivityResult
    // lint check (it can't tell our ComponentActivity registerForActivityResult
    // usage is safe). Force a current fragment (>= 1.3.0) to clear it.
    implementation("androidx.fragment:fragment:1.8.5")

    val composeBom = platform("androidx.compose:compose-bom:2024.10.01")
    implementation(composeBom)
    implementation("androidx.compose.ui:ui")
    implementation("androidx.compose.ui:ui-graphics")
    implementation("androidx.compose.ui:ui-tooling-preview")
    implementation("androidx.compose.material3:material3")
    implementation("androidx.compose.material:material-icons-extended")

    // Google's "Code Scanner" — bundled scanner UI for QR / barcode imports
    // on AddScreen. Pulled via Play Services Module Install API, no CAMERA
    // permission needed; first launch may prompt to install the module
    // (silent on most devices, ~1s).
    implementation("com.google.android.gms:play-services-code-scanner:16.1.0")

    debugImplementation("androidx.compose.ui:ui-tooling")

    testImplementation("junit:junit:4.13.2")
}
