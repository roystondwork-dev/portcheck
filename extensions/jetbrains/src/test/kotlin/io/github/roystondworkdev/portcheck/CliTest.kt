package io.github.roystondworkdev.portcheck

import org.junit.Assert.assertEquals
import org.junit.Assert.assertThrows
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.File

class CliTest {
    private class FakeExec(private val result: ExecResult) : Exec {
        var args: List<String> = emptyList()

        override fun run(file: String, args: List<String>, timeoutMs: Long): ExecResult {
            this.args = args
            return result
        }
    }

    private fun client(stdout: String, code: Int = 0, stderr: String = "") = FakeExec(ExecResult(stdout, stderr, code)).let { it to PortcheckClient("portcheck", it) }

    private fun envelope(kind: String, data: String) = """{"schemaVersion":1,"kind":"$kind","data":$data}"""

    @Test
    fun listParsesSockets() {
        val (exec, c) = client(
            envelope(
                "list",
                """{"sockets":[{"proto":"tcp","localAddress":"127.0.0.1","localPort":3000,"state":"LISTEN","pid":42,
                   "process":{"pid":42,"name":"node","user":"alice","ancestors":[{"pid":7,"name":"npm"}]}}],"partial":true}""",
            ),
        )
        val data = c.list()
        assertEquals(listOf("list", "--listening", "--json"), exec.args)
        assertTrue(data.partial)
        val s = data.sockets.single()
        assertEquals(3000, s.localPort)
        assertEquals("node", s.process?.name)
        assertEquals(listOf(ProcessRef(7, "npm")), s.process?.ancestors)
    }

    @Test
    fun checkAcceptsExitCode1() {
        val (exec, c) = client(envelope("check", """{"ports":[{"port":3000,"proto":"tcp","status":"in-use","holders":[]}]}"""), code = 1)
        assertEquals("in-use", c.check(3000).ports.single().status)
        assertEquals(listOf("check", "3000", "--json"), exec.args)
    }

    @Test
    fun freeKillAndVersion() {
        assertEquals(listOf(3001), client(envelope("free", """{"ports":[3001]}""")).second.free("3000-3999").ports)
        val (exec, c) = client(envelope("kill", """{"results":[{"pid":42,"name":"node","outcome":"terminated"}]}"""), code = 1)
        assertEquals("terminated", c.kill(42, force = true).results.single().outcome)
        assertEquals(listOf("kill", "--pid", "42", "--yes", "--force", "--json"), exec.args)
        assertEquals("v0.9.0", client(envelope("version", """{"version":"v0.9.0"}""")).second.version().version)
    }

    @Test
    fun errorsSayWhatWentWrong() {
        fun message(stdout: String, code: Int = 0, stderr: String = "") =
            assertThrows(PortcheckException::class.java) { client(stdout, code, stderr).second.list() }.message

        assertEquals("unknown flag: --nope", message("", 2, "\nportcheck: unknown flag: --nope\nRun 'portcheck --help'"))
        assertEquals("portcheck exited with status 3", message("", 3))
        assertEquals("portcheck printed invalid JSON for \"list\"", message("{not json"))
        assertEquals("portcheck printed invalid JSON for \"list\"", message("[1]"))
        assertEquals(
            "portcheck JSON schema 2 is not supported (expected 1); update portcheck or the plugin",
            message("""{"schemaVersion":2,"kind":"list","data":{}}"""),
        )
        assertEquals("expected \"list\" output from portcheck, got \"check\"", message(envelope("check", "{}")))
    }

    @Test
    fun platformDirs() {
        assertEquals("darwin-arm64", platformDir("Mac OS X", "aarch64"))
        assertEquals("linux-amd64", platformDir("Linux", "amd64"))
        assertEquals("windows-amd64", platformDir("Windows 11", "amd64"))
        assertEquals("windows-arm64", platformDir("Windows 11", "arm64"))
    }

    @Test
    fun resolveBinaryPrefersSettingThenBundledThenPath() {
        val dir = File("/plugins/portcheck-jetbrains")
        val bundled = File("/plugins/portcheck-jetbrains/bin/darwin-arm64/portcheck")
        assertEquals("/opt/pc", resolveBinary(" /opt/pc ", dir, "Mac OS X", "aarch64") { true })
        assertEquals(bundled.path, resolveBinary("", dir, "Mac OS X", "aarch64") { it == bundled })
        assertEquals("portcheck", resolveBinary("", dir, "Mac OS X", "x86_64") { it == bundled })
        assertEquals("portcheck.exe", resolveBinary("", null, "Windows 11", "amd64") { true })
    }

    @Test
    fun processExecRunsRealPrograms() {
        val sh = if (System.getProperty("os.name").startsWith("Windows")) return else "/bin/sh"
        assertEquals(ExecResult("out\n", "err\n", 3), ProcessExec.run(sh, listOf("-c", "echo out; echo err >&2; exit 3"), 5_000))
        assertEquals(
            "portcheck didn't answer within 0 s",
            assertThrows(PortcheckException::class.java) { ProcessExec.run(sh, listOf("-c", "sleep 5"), 200) }.message,
        )
        assertTrue(
            assertThrows(PortcheckException::class.java) { ProcessExec.run("/nonexistent/portcheck", emptyList(), 1_000) }.message!!
                .startsWith("portcheck couldn't be started"),
        )
    }

    @Test
    fun processExecMakesTheBundledBinaryExecutable() {
        if (System.getProperty("os.name").startsWith("Windows")) return
        val f = File.createTempFile("portcheck", ".sh").apply {
            writeText("#!/bin/sh\necho ok\n")
            setExecutable(false)
            deleteOnExit()
        }
        assertEquals("ok\n", ProcessExec.run(f.path, emptyList(), 5_000).stdout)
    }
}
