import org.jetbrains.intellij.platform.gradle.TestFrameworkType

plugins {
    id("org.jetbrains.kotlin.jvm")
    id("org.jetbrains.intellij.platform")
    id("org.jetbrains.kotlinx.kover")
}

group = "io.github.roystondworkdev.portcheck"
version = providers.gradleProperty("pluginVersion").get()

kotlin {
    jvmToolchain(21)
}

dependencies {
    testImplementation("junit:junit:4.13.2")

    intellijPlatform {
        intellijIdea(providers.gradleProperty("platformVersion"))
        testFramework(TestFrameworkType.Platform)
        pluginVerifier()
    }
}

intellijPlatform {
    pluginConfiguration {
        version = providers.gradleProperty("pluginVersion")
        ideaVersion {
            sinceBuild = providers.gradleProperty("pluginSinceBuild")
            untilBuild = provider { null } // no upper bound: new IDE versions keep the plugin
        }
    }
    pluginVerification {
        ides {
            recommended()
        }
    }
    signing {
        certificateChain = providers.environmentVariable("JETBRAINS_CERTIFICATE_CHAIN")
        privateKey = providers.environmentVariable("JETBRAINS_PRIVATE_KEY")
        password = providers.environmentVariable("JETBRAINS_PRIVATE_KEY_PASSWORD")
    }
    publishing {
        token = providers.environmentVariable("JETBRAINS_MARKETPLACE_TOKEN")
    }
}

// Release builds bundle portcheck for every OS and CPU, as the VS Code
// extension does: -PportcheckBin=<dir with darwin-arm64/portcheck, …>.
// Without it, the plugin uses portcheck from PATH.
val portcheckBin = providers.gradleProperty("portcheckBin")
tasks.prepareSandbox {
    if (portcheckBin.isPresent) {
        from(portcheckBin) {
            into("${rootProject.name}/bin")
        }
    }
}
tasks.buildPlugin {
    // Keep the binaries executable in the zip (the plugin also fixes this at runtime).
    eachFile {
        if ("/bin/" in path) {
            permissions { unix("rwxr-xr-x") }
        }
    }
}

kover {
    reports {
        filters {
            // Swing UI and IDE wiring; covered by the platform tests and a manual check.
            excludes {
                packages("io.github.roystondworkdev.portcheck.ui")
            }
        }
        verify {
            rule {
                minBound(95)
            }
        }
    }
}
