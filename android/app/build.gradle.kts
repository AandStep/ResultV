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
        versionCode = 10000
        versionName = "1.0.0"
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
            if (hasReleaseKeystore) {
                signingConfig = signingConfigs.getByName("release")
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
    splits {
        abi {
            isEnable = true
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
}
