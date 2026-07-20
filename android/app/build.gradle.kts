import java.io.File

plugins {
    alias(libs.plugins.android.application)
    alias(libs.plugins.kotlin.android)
    alias(libs.plugins.kotlin.compose)
    alias(libs.plugins.kotlin.serialization)
}

// Fail fast if the AAR hasn't been built yet.
val coreAar = rootProject.file("core/core.aar")
tasks.register("checkCoreAar") {
    doFirst {
        check(coreAar.exists()) {
            "core/core.aar not found. Run `make android-core` from the repo root first."
        }
    }
}
tasks.named("preBuild") { dependsOn("checkCoreAar") }

android {
    namespace = "com.cristim.dailyprogress"
    compileSdk = 35

    defaultConfig {
        applicationId = "com.cristim.dailyprogress"
        minSdk = 26
        targetSdk = 35
        versionCode = 1
        versionName = "0.1.0"

        testInstrumentationRunner = "androidx.test.runner.AndroidJUnitRunner"

        // Google OAuth Android client ID — replace before enabling sync.
        buildConfigField("String", "GOOGLE_CLIENT_ID", "\"\"")

        // Only include the two ABIs needed for device + emulator in debug builds.
        ndk {
            abiFilters += listOf("arm64-v8a", "x86_64")
        }
    }

    buildTypes {
        release {
            isMinifyEnabled = false
            proguardFiles(
                getDefaultProguardFile("proguard-android-optimize.txt"),
                "proguard-rules.pro"
            )
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

    sourceSets {
        getByName("main") {
            kotlin.srcDirs("src/main/kotlin")
        }
        getByName("test") {
            kotlin.srcDirs("src/test/kotlin")
        }
    }
}

dependencies {
    // Go core AAR (produced by `make android-core` from the repo root).
    // Local file deps are supported in application modules under AGP 8.x.
    implementation(files("../core/core.aar"))

    // Kotlin serialization for JSON decoding of Core API responses.
    implementation(libs.kotlinx.serialization.json)

    // Jetpack Compose (BOM governs all compose-* artifact versions).
    val composeBom = platform(libs.compose.bom)
    implementation(composeBom)
    implementation(libs.compose.ui)
    implementation(libs.compose.ui.graphics)
    implementation(libs.compose.ui.tooling.preview)
    implementation(libs.compose.material3)
    implementation(libs.compose.material.icons.extended)
    debugImplementation(libs.compose.ui.tooling)

    // Navigation + Lifecycle
    implementation(libs.navigation.compose)
    implementation(libs.lifecycle.viewmodel.compose)
    implementation(libs.lifecycle.runtime.compose)

    // Google sign-in (AppAuth PKCE) and encrypted token storage.
    implementation(libs.appauth)
    implementation(libs.security.crypto)

    // WorkManager placeholder (v2 background sync — wired but not used in v1).
    implementation(libs.workmanager)

    // Tests
    testImplementation(libs.junit)
    testImplementation(libs.kotlinx.coroutines.test)
    androidTestImplementation(libs.androidx.test.ext.junit)
}
