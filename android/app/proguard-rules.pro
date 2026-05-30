# gomobile-generated bindings rely on JNI lookups by exact class/method name.
# Without this, R8 will rename mobile.Mobile and the native side won't find it.
-keep class mobile.** { *; }
-keep class libbox.** { *; }
-keep class go.** { *; }

# Go runtime callbacks — gomobile registers these via JNI from native code.
# R8 can't see the native references and would strip them.
-keep class org.golang.app.** { *; }

# Keep Compose @Composable metadata — R8 can strip it in aggressive mode,
# breaking recomposition. The kotlin-compose compiler plugin emits synthetic
# classes that must survive minification.
-keepclassmembers class ** {
    @androidx.compose.runtime.Composable <methods>;
}

# Google Play Services Code Scanner — reflection-heavy, protect the API surface.
-keep class com.google.mlkit.** { *; }
-keep class com.google.android.gms.internal.mlkit_vision_** { *; }
