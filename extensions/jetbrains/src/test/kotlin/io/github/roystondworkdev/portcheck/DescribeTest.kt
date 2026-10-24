package io.github.roystondworkdev.portcheck

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

class DescribeTest {
    private val node = Socket(
        proto = "tcp", localAddress = "127.0.0.1", localPort = 3000, state = "LISTEN", pid = 42,
        process = ProcessInfo(pid = 42, name = "node", user = "alice", ancestors = listOf(ProcessRef(7, "npm"), ProcessRef(3, "zsh"))),
    )
    private val web = Socket(proto = "tcp", localPort = 8080, container = ContainerInfo(id = "abc", name = "web", runtime = "docker", stopHint = "docker stop web"))
    private val hidden = Socket(proto = "tcp", localPort = 22)

    private fun check(port: Int, status: String, vararg holders: Socket) = CheckData(listOf(PortStatus(port, "tcp", status, holders.toList())))

    @Test
    fun busyPortNamesTheProcessAndOffersToStopIt() {
        assertEquals(
            BusyPort(3000, "Port 3000 is in use by node (pid 42), started by npm (7) ← zsh (3).", listOf(Fix.Stop(42, "node"))),
            busyPort(check(3000, "in-use", node, node), 3000),
        )
    }

    @Test
    fun busyPortForContainersHiddenOwnersAndSeveralHolders() {
        assertEquals(listOf(Fix.Container("docker stop web")), busyPort(check(8080, "in-use", web), 8080)?.fixes)
        assertEquals("Port 8080 is in use by docker:web.", busyPort(check(8080, "in-use", web), 8080)?.summary)
        assertEquals(
            BusyPort(22, "Port 22 is in use by a process hidden from your user.", listOf(Fix.Hidden)),
            busyPort(check(22, "in-use", hidden), 22),
        )
        val two = busyPort(check(3000, "in-use", node, node.copy(pid = 43, process = ProcessInfo(43, "java"))), 3000)
        assertEquals("Port 3000 is in use by node (pid 42), started by npm (7) ← zsh (3) and 1 more.", two?.summary)
        assertEquals(listOf(Fix.Stop(42, "node"), Fix.Stop(43, "java")), two?.fixes)
        assertEquals("Port 1 is in use by another process.", busyPort(check(1, "in-use"), 1)?.summary)
    }

    @Test
    fun busyPortIsNullWhenFreeOrUnknown() {
        assertNull(busyPort(check(3000, "free"), 3000))
        assertNull(busyPort(check(3000, "in-use", node), 4000))
        assertEquals(BusyPort(80, "Port 80 can't be used by your user.", emptyList()), busyPort(check(80, "denied"), 80))
    }

    @Test
    fun holderNames() {
        assertEquals("docker:abc", holderName(web.copy(container = ContainerInfo(id = "abc", runtime = "docker"))))
        assertEquals("podman:redis", holderName(web.copy(container = ContainerInfo(image = "redis", runtime = "podman"))))
        assertEquals("docker:?", holderName(web.copy(container = ContainerInfo(runtime = "docker"))))
        assertEquals("process (pid 5)", holderName(Socket(pid = 5)))
        assertEquals("", ancestry(null))
    }

    @Test
    fun killSummaries() {
        val ok = KillData(listOf(KillResult(42, "node", "terminated"), KillResult(43, null, "already-exited")))
        assertEquals("node (pid 42): terminated\nprocess (pid 43): already-exited", killSummary(ok))
        assertTrue(killSucceeded(ok))
        val failed = KillData(listOf(KillResult(1, "launchd", "failed", "refusing to stop a system process")))
        assertEquals("launchd (pid 1): couldn't be stopped: refusing to stop a system process", killSummary(failed))
        assertEquals("process (pid 2): couldn't be stopped: unknown error", killSummary(KillData(listOf(KillResult(2, outcome = "failed")))))
        assertFalse(killSucceeded(failed))
        assertFalse(killSucceeded(KillData()))
    }

    @Test
    fun rowsAreUniqueSortedAndFilterable() {
        val rows = portRows(ListData(listOf(web, node, node, hidden, node.copy(proto = "udp", process = null))))
        assertEquals(
            listOf(
                PortRow(22, "tcp", "", "a process hidden from your user", null, "", "http://localhost:22/"),
                PortRow(3000, "tcp", "127.0.0.1", "node", 42, "alice", "http://localhost:3000/"),
                PortRow(3000, "udp", "127.0.0.1", "?", 42, "", "http://localhost:3000/"),
                PortRow(8080, "tcp", "", "docker:web", null, "", "http://localhost:8080/"),
            ),
            rows,
        )
        assertEquals(rows, filterRows(rows, "  "))
        assertEquals(listOf(3000, 3000), filterRows(rows, "300").map { it.port })
        assertEquals(listOf(3000), filterRows(rows, "NODE").map { it.port })
        assertEquals(listOf(3000), filterRows(rows, "alice").map { it.port })
        assertEquals(emptyList<PortRow>(), filterRows(rows, "nothing"))
    }
}
