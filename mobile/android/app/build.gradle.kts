plugins {
    id("com.android.application")
    id("org.jetbrains.kotlin.android")
}

android {
    namespace = "com.lsyl.tunnel.mobile"
    compileSdk = 35

    defaultConfig {
        applicationId = "com.lsyl.tunnel.mobile"
        minSdk = (findProperty("minSdkOverride") as String?)?.toIntOrNull() ?: 29
        targetSdk = 35
        versionCode = 1
        versionName = "1.0.0"
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }

    kotlinOptions {
        jvmTarget = "17"
    }
}
