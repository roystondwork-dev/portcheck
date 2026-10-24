// A thin client for the portcheck CLI's JSON output (ADR-0004). No port logic
// lives here: every call runs `portcheck … --json` and validates the result.
package io.github.roystondworkdev.portcheck

import com.google.gson.Gson
import com.google.gson.JsonParseException
import com.google.gson.JsonParser
import java.io.File
import java.util.concurrent.TimeUnit

data class ExecResult(val stdout: String, val stderr: String, val code: Int)

/** Runs a program and returns its output and exit code. */
fun interface Exec {
    fun run(file: String, args: List<String>, timeoutMs: Long): ExecResult
}

class PortcheckException(message: String, val exitCode: Int? = null) : Exception(message)

/** The OS and CPU in Go's names, as release archives use them (darwin-arm64, windows-amd64, …). */
fun platformDir(osName: String, osArch: String): String {
    val os = when {
        osName.startsWith("Mac", ignoreCase = true) -> "darwin"
        osName.startsWith("Windows", ignoreCase = true) -> "windows"
        else -> "linux"
    }
    val arch = if (osArch == "aarch64" || osArch == "arm64") "arm64" else "amd64"
    return "$os-$arch"
}

/**
 * Picks the binary: the setting, else the one bundled for this OS and CPU in
 * bin/<os>-<arch>/ (release builds carry all six), else portcheck on PATH.
 */
fun resolveBinary(configured: String, pluginDir: File?, osName: String, osArch: String, exists: (File) -> Boolean): String {
    if (configured.isNotBlank()) {
        return configured.trim()
    }
    val exe = if (osName.startsWith("Windows", ignoreCase = true)) "portcheck.exe" else "portcheck"
    val bundled = pluginDir?.resolve("bin")?.resolve(platformDir(osName, osArch))?.resolve(exe)
    return if (bundled != null && exists(bundled)) bundled.path else exe
}

/** Runs a real process. */
object ProcessExec : Exec {
    override fun run(file: String, args: List<String>, timeoutMs: Long): ExecResult {
        val bin = File(file)
        if (bin.isFile && !bin.canExecute()) {
            bin.setExecutable(true) // plugin zips don't keep the executable bit
        }
        val p = try {
            ProcessBuilder(listOf(file) + args).start()
        } catch (e: java.io.IOException) {
            throw PortcheckException("portcheck couldn't be started: ${e.message}")
        }
        // Read both streams while waiting, so a full pipe can't block portcheck
        // and a hung portcheck can't block us past the timeout.
        var stdout = ""
        var stderr = ""
        val readers = listOf(
            Thread { stdout = p.inputStream.bufferedReader().readText() },
            Thread { stderr = p.errorStream.bufferedReader().readText() },
        ).onEach { it.start() }
        if (!p.waitFor(timeoutMs, TimeUnit.MILLISECONDS)) {
            p.destroyForcibly()
            throw PortcheckException("portcheck didn't answer within ${timeoutMs / 1000} s")
        }
        readers.forEach { it.join() }
        return ExecResult(stdout, stderr, p.exitValue())
    }
}

class PortcheckClient(private val binary: String, private val exec: Exec, private val timeoutMs: Long = 15_000) {
    private val gson = Gson()

    /**
     * Runs portcheck with --json. Exit codes in okCodes are expected outcomes
     * (e.g. 1 = "port in use" for check). Output must be a schema v1 envelope
     * of the expected kind.
     */
    fun <T> run(kind: String, type: Class<T>, args: List<String>, okCodes: Set<Int> = setOf(0)): T {
        val r = exec.run(binary, args + "--json", timeoutMs)
        val out = r.stdout.trim()
        if (r.code !in okCodes || out.isEmpty()) {
            throw PortcheckException(firstLine(r.stderr) ?: "portcheck exited with status ${r.code}", r.code)
        }
        val env = try {
            JsonParser.parseString(out).asJsonObject
        } catch (e: JsonParseException) {
            throw PortcheckException("portcheck printed invalid JSON for \"$kind\"", r.code)
        } catch (e: IllegalStateException) {
            throw PortcheckException("portcheck printed invalid JSON for \"$kind\"", r.code)
        }
        val version = env.get("schemaVersion")?.takeIf { it.isJsonPrimitive }?.asInt
        if (version != SCHEMA_VERSION) {
            throw PortcheckException("portcheck JSON schema $version is not supported (expected $SCHEMA_VERSION); update portcheck or the plugin", r.code)
        }
        val got = env.get("kind")?.takeIf { it.isJsonPrimitive }?.asString
        if (got != kind) {
            throw PortcheckException("expected \"$kind\" output from portcheck, got \"$got\"", r.code)
        }
        return gson.fromJson(env.get("data"), type)
    }

    fun list(): ListData = run("list", ListData::class.java, listOf("list", "--listening"))

    fun check(port: Int): CheckData = run("check", CheckData::class.java, listOf("check", port.toString()), setOf(0, 1))

    fun free(range: String, count: Int = 1): FreeData =
        run("free", FreeData::class.java, listOf("free", range, "-n", count.toString()), setOf(0, 1))

    /** Stops a process. Exit codes 1, 3 and 4 still carry per-process results. */
    fun kill(pid: Int, force: Boolean = false): KillData {
        val args = mutableListOf("kill", "--pid", pid.toString(), "--yes")
        if (force) {
            args += "--force"
        }
        return run("kill", KillData::class.java, args, setOf(0, 1, 3, 4))
    }

    fun version(): VersionData = run("version", VersionData::class.java, listOf("version"))
}

private fun firstLine(s: String): String? =
    s.lineSequence().map { it.trim() }.firstOrNull { it.isNotEmpty() }?.removePrefix("portcheck: ")
