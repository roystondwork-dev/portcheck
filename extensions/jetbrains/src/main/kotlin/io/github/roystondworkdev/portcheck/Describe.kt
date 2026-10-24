// Plain-text descriptions of portcheck's results, for the tool window and
// notifications. Pure; unit-tested.
package io.github.roystondworkdev.portcheck

/** Who holds a socket: "node (pid 42)", "docker:web", or "a hidden process". */
fun holderName(s: Socket): String = when {
    s.container != null -> "${s.container.runtime}:${s.container.name ?: s.container.image ?: s.container.id ?: "?"}"
    s.pid == null -> "a process hidden from your user"
    else -> "${s.process?.name ?: "process"} (pid ${s.pid})"
}

/** "npm (7) ← zsh (3)", or "". */
fun ancestry(p: ProcessInfo?): String = p?.ancestors.orEmpty().joinToString(" ← ") { "${it.name} (${it.pid})" }

/** What to do about a busy port, as the notification offers it. */
sealed interface Fix {
    /** Stop this process (portcheck's guards apply). */
    data class Stop(val pid: Int, val name: String) : Fix

    /** A container: never stop the runtime's proxy; show the command instead. */
    data class Container(val stopHint: String) : Fix

    /** The owner is hidden: portcheck needs elevated privileges to see or stop it. */
    data object Hidden : Fix
}

data class BusyPort(val port: Int, val summary: String, val fixes: List<Fix>)

/** Describes a check result for one port; null when the port is free. */
fun busyPort(data: CheckData, port: Int): BusyPort? {
    val p = data.ports.firstOrNull { it.port == port } ?: return null
    if (p.status == "free") {
        return null
    }
    if (p.status == "denied") {
        return BusyPort(port, "Port $port can't be used by your user.", emptyList())
    }
    val holders = p.holders.distinctBy { it.pid to it.container?.id }
    val fixes = holders.map { s ->
        when {
            s.container != null -> Fix.Container(s.container.stopHint)
            s.pid == null -> Fix.Hidden
            else -> Fix.Stop(s.pid, s.process?.name ?: "process")
        }
    }.distinct()
    val first = holders.firstOrNull()
    val who = if (first == null) "another process" else holderName(first)
    val started = ancestry(first?.process).let { if (it.isEmpty()) "" else ", started by $it" }
    val more = if (holders.size > 1) " and ${holders.size - 1} more" else ""
    return BusyPort(port, "Port $port is in use by $who$started$more.", fixes)
}

/** One line per stopped process, e.g. "node (pid 42): terminated". */
fun killSummary(data: KillData): String = data.results.joinToString("\n") { r ->
    val who = "${r.name ?: "process"} (pid ${r.pid})"
    if (r.outcome == "failed") "$who: couldn't be stopped: ${r.error ?: "unknown error"}" else "$who: ${r.outcome}"
}

/** True when every process was stopped (or had already exited). */
fun killSucceeded(data: KillData): Boolean = data.results.isNotEmpty() && data.results.none { it.outcome == "failed" }

/** Rows for the Ports tool window: listening sockets, one per port and process. */
data class PortRow(val port: Int, val proto: String, val address: String, val process: String, val pid: Int?, val user: String, val url: String)

fun portRows(data: ListData): List<PortRow> = data.sockets
    .map { s ->
        PortRow(
            port = s.localPort,
            proto = s.proto,
            address = s.localAddress,
            process = if (s.container != null || s.pid == null) holderName(s) else s.process?.name ?: "?",
            pid = s.pid,
            user = s.process?.user.orEmpty(),
            url = "http://localhost:${s.localPort}/",
        )
    }
    .distinctBy { Triple(it.port, it.proto, it.pid) }
    .sortedWith(compareBy({ it.port }, { it.proto }))

/** Rows matching a filter: a port number, process name or user. */
fun filterRows(rows: List<PortRow>, filter: String): List<PortRow> {
    val q = filter.trim().lowercase()
    if (q.isEmpty()) {
        return rows
    }
    return rows.filter { q in it.port.toString() || q in it.process.lowercase() || q in it.user.lowercase() }
}
