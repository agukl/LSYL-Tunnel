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
        versionCode = 4
        versionName = "2.1.0"
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }

    kotlinOptions {
        jvmTarget = "17"
    }
}

dependencies {
    implementation("org.snakeyaml:snakeyaml-engine:3.0.1")
    testImplementation("junit:junit:4.13.2")
    testImplementation("org.json:json:20240303")
}
