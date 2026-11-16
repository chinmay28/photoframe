plugins {
    id("com.android.application")
    id("org.jetbrains.kotlin.android")
}

// Photoframe for Android is a thin client only: the frame's Go server drives
// the panel's hardware (SPI, GPIO, the A–D buttons) on the Pi, so there is no
// server for the phone to run and no Go in this build.

android {
    namespace = "io.github.chinmay28.photoframe"
    compileSdk = 35

    defaultConfig {
        applicationId = "io.github.chinmay28.photoframe"
        // Android 10: ImageDecoder for HEIC shares, and the AndroidX web view
        // features the page bridge uses on every current web view.
        minSdk = 29
        targetSdk = 35
        // Stamped by the release workflow: the release's version
        // (scripts/version.sh) as the name, its commit count as the code,
        // which only grows.
        versionCode = (findProperty("appVersionCode") as String?)?.toInt() ?: 1
        versionName = (findProperty("appVersionName") as String?) ?: "dev"
    }

    buildFeatures {
        buildConfig = true
    }

    buildTypes {
        release {
            isMinifyEnabled = false
            // Signed with the debug key so the release build sideloads as it
            // stands. Anybody publishing to a store signs it with their own.
            signingConfig = signingConfigs.getByName("debug")
        }
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }
    kotlinOptions {
        jvmTarget = "17"
    }
    lint {
        disable += setOf(
            // Plain HTTP is how the frame runs on a home network, and a
            // private CA is how it would run behind TLS at home: both are
            // the point (see network_security_config.xml).
            "InsecureBaseConfiguration",
            "AcceptsUserCertificates",
            // allowBackup is off; there is nothing for fullBackupContent to say.
            "DataExtractionRules",
            // Versions move on purpose, together, not whenever lint notices.
            "GradleDependency",
        )
    }
}

dependencies {
    implementation(project(":core"))
    implementation("androidx.core:core-ktx:1.13.1")
    implementation("androidx.activity:activity-ktx:1.9.3")
    implementation("androidx.appcompat:appcompat:1.7.0")
    implementation("androidx.preference:preference-ktx:1.2.1")
    implementation("androidx.webkit:webkit:1.12.1")
    implementation("com.google.android.material:material:1.12.0")
}
