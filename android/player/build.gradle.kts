plugins { id("com.android.application"); id("org.jetbrains.kotlin.android") }
android {
    namespace = "com.phoring.fenghuangming.player"
    compileSdk = 36
    compileSdkExtension = 19
    buildToolsVersion = "36.1.0"
    defaultConfig {
        applicationId = "com.phoring.fenghuangming.player"
        minSdk = 29
        targetSdk = 36
        versionCode = 20003
        versionName = "0.2.0-android.3"
    }
    signingConfigs {
        create("module") {
            storeFile = file(System.getenv("FHM_KEYSTORE") ?: "../.signing/player.jks")
            storePassword = System.getenv("FHM_STORE_PASSWORD")
            keyAlias = "fenghuangming"
            keyPassword = System.getenv("FHM_STORE_PASSWORD")
        }
    }
    buildTypes { getByName("release") { isMinifyEnabled = false; signingConfig = signingConfigs.getByName("module") } }
    compileOptions { sourceCompatibility = JavaVersion.VERSION_17; targetCompatibility = JavaVersion.VERSION_17 }
}
kotlin {
    jvmToolchain(21)
    compilerOptions { jvmTarget.set(org.jetbrains.kotlin.gradle.dsl.JvmTarget.JVM_17) }
}
dependencies {
    implementation("androidx.media3:media3-exoplayer:1.8.1")
    implementation("androidx.media3:media3-session:1.8.1")
    testImplementation("junit:junit:4.13.2")
}
