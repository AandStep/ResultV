import java.util.Properties

plugins {
    id("com.android.application")
    id("org.jetbrains.kotlin.android")
    id("org.jetbrains.kotlin.plugin.compose")
}

// PoC test URI lives in local.properties (gitignored) so we don't leak
// credentials. Read once at configuration time and surface via BuildConfig.
val pocVlessUri: String = run {
    val props = Properties()
    val f = rootProject.file("local.properties")
    if (f.exists()) {
        f.inputStream().use { props.load(it) }
    }
    props.getProperty("vless.uri", "")
}

android {
    namespace = "com.resultv.android"
    compileSdk = 34

    defaultConfig {
        applicationId = "com.resultv.android"
        minSdk = 26
        targetSdk = 34
        versionCode = 1
        versionName = "0.2.0-poc"

        buildConfigField("String", "VLESS_URI", "\"${pocVlessUri.replace("\"", "\\\"")}\"")
    }

    buildTypes {
        release {
            isMinifyEnabled = false
            // Release ships every ABI so phones (arm64-v8a) and emulators
            // (x86_64) both run. Adds ~80 MB versus debug.
            ndk { abiFilters += listOf("arm64-v8a", "armeabi-v7a", "x86", "x86_64") }
            proguardFiles(
                getDefaultProguardFile("proguard-android-optimize.txt"),
                "proguard-rules.pro"
            )
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
