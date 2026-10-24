package io.github.roystondworkdev.portcheck

import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Assume.assumeTrue
import org.junit.Test
import java.io.File
import java.net.InetAddress
import java.net.ServerSocket

/**
 * Against the real portcheck binary: PORTCHECK_BIN=<path> (CI builds it;
 * locally `make build` puts it in bin/). Skipped when unset.
 */
class RealPortcheckTest {
    private val bin = System.getenv("PORTCHECK_BIN").orEmpty()

    @Test
    fun namesTheProcessHoldingAPort() {
        assumeTrue("PORTCHECK_BIN not set", bin.isNotEmpty() && File(bin).isFile)
        val c = PortcheckClient(bin, ProcessExec)
        assertTrue(c.version().version.isNotEmpty())
        ServerSocket(0, 50, InetAddress.getLoopbackAddress()).use { server ->
            val port = server.localPort
            val busy = busyPort(c.check(port), port)
            val me = ProcessHandle.current().pid().toInt()
            assertEquals(port, busy?.port)
            assertTrue(busy?.summary, busy!!.fixes.contains(Fix.Stop(me, ProcessHandle.current().info().command().get().substringAfterLast(File.separatorChar).removeSuffix(".exe"))))
            assertTrue(portRows(c.list()).any { it.port == port && it.pid == me })
        }
        val free = c.free("40000-40100").ports.single()
        assertEquals(null, busyPort(c.check(free), free))
    }
}
