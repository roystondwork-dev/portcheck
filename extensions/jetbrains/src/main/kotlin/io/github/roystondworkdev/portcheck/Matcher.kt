// Recognises "port already in use" errors printed by dev servers, databases
// and tools in Run/Debug consoles. The patterns match the VS Code extension's
// (extensions/vscode/src/matcher.ts); keep the two in sync.
package io.github.roystondworkdev.portcheck

data class PortError(
    val port: Int,
    /** What printed it, e.g. "Node.js"; for the notification and the log. */
    val source: String,
)

private class Pattern(val source: String, val re: Regex)

private fun p(source: String, re: String, ignoreCase: Boolean = false) =
    Pattern(source, if (ignoreCase) Regex(re, RegexOption.IGNORE_CASE) else Regex(re))

// Each pattern captures the port in the named group "port". Order matters:
// the first matching pattern wins.
private val PATTERNS = listOf(
    // Error: listen EADDRINUSE: address already in use :::3000
    p("Node.js", """EADDRINUSE(?::? address already in use)?:? (?:\S*:)?(?<port>\d{1,5})\b"""),
    p("Node.js", """code: 'EADDRINUSE'.*\bport: (?<port>\d{1,5})\b"""),
    // listen tcp :8080: bind: address already in use
    p("Go", """listen (?:tcp|udp)[46]? \S*?:(?<port>\d{1,5}): bind: (?:address already in use|Only one usage of each socket address)""", true),
    p("Docker", """Bind for \S*:(?<port>\d{1,5}) failed: port is already allocated"""),
    p("Ruby", """Address already in use - bind\(2\) for "?[^"\s]*"? port (?<port>\d{1,5})"""),
    p("Ruby", """Address already in use - bind\(2\) for \S*:(?<port>\d{1,5})"""),
    // Web server failed to start. Port 8080 was already in use.
    p("Spring Boot", """Port (?<port>\d{1,5}) was already in use"""),
    p(".NET", """Failed to bind to address \S*?:(?<port>\d{1,5})/?:? address already in use""", true),
    p("Python", """error while attempting to bind on address \('[^']*', (?<port>\d{1,5})\)"""),
    p("Flask", """Port (?<port>\d{1,5}) is in use by another program"""),
    p("PHP", """Failed to listen on \S*:(?<port>\d{1,5}) \(reason: Address already in use\)"""),
    p("nginx", """bind\(\) to \S*:(?<port>\d{1,5}) failed \(\d+: Address already in use\)"""),
    p("Redis", """listening socket \S*:(?<port>\d{1,5}): bind: Address already in use"""),
    p("PostgreSQL", """another postmaster already running on port (?<port>\d{1,5})"""),
    p("Create React App", """Something is already running on port (?<port>\d{1,5})"""),
    p("Bun", """Failed to start server\. Is port (?<port>\d{1,5}) in use\?"""),
    p("dev server", """\bPort (?<port>\d{1,5}) is already in use""", true),
    p("dev server", """\bPort (?<port>\d{1,5}) is in use, trying""", true),
    // Java: java.net.BindException: Address already in use (no port; Tomcat names it below)
    p("Tomcat", """Failed to (?:start|initialize) (?:end ?point|connector)[^\n]*?\[?(?:http|https|ajp)-\S*?-(?<port>\d{1,5})\]?""", true),
    p("unknown", """address already in use\S*\s+\S*?:(?<port>\d{1,5})\b""", true),
    p("unknown", """\bport (?<port>\d{1,5})\b[^.]*\balready in use""", true),
)

// Errors that don't say which port: the port comes from the command that failed.
private val PORTLESS = listOf(
    p("Python", """\[Errno (?:48|98|10048)\] Address already in use"""),
    p("Django", """That port is already in use"""),
    p("Deno", """AddrInUse: Address already in use"""),
    p("Rust", """kind: AddrInUse"""),
    p("Java", """java\.net\.BindException: Address already in use"""),
    p("unknown", """Address already in use \(os error \d+\)""", true),
)

// Default ports of tools, for commands that don't name one.
private val DEFAULT_PORTS = listOf(
    Regex("""\bmanage\.py runserver\b""") to 8000,
    Regex("""\bhttp\.server\b""") to 8000,
    Regex("""\bflask run\b""") to 5000,
    Regex("""\b(?:rails (?:s|server)|next (?:dev|start)|react-scripts start)\b""") to 3000,
    Regex("""\bvite\b""") to 5173,
    Regex("""\bng serve\b""") to 4200,
    Regex("""\bastro dev\b""") to 4321,
    Regex("""\bhugo server\b""") to 1313,
    Regex("""\bjekyll serve\b""") to 4000,
    // Spring Boot's default
    Regex("""\bspring-boot:run\b|\bbootRun\b""") to 8080,
)

private val NAMED_PORT = Regex("""(?:--port[= ]|-p[= ]?|--bind[= ]\S*:|\s\S*:)(?<port>\d{2,5})\b""")
private val BARE_PORT = Regex("""\s(?<port>\d{2,5})(?=\s|$)""")

private fun valid(port: Int?) = port != null && port in 1..65535

/** The port a command will listen on, when it names one or the tool has a known default. */
fun portFromCommand(cmd: String): Int? {
    val named = (NAMED_PORT.find(cmd) ?: BARE_PORT.find(cmd))?.groups?.get("port")?.value?.toIntOrNull()
    if (valid(named)) {
        return named
    }
    return DEFAULT_PORTS.firstOrNull { (re, _) -> re.containsMatchIn(cmd) }?.second
}

/**
 * Finds a "port in use" error in one line of output. For errors that don't
 * name the port, commandLine (the command that printed it) supplies it.
 */
fun matchPortError(line: String, commandLine: String? = null): PortError? {
    for (pattern in PATTERNS) {
        val port = pattern.re.find(line)?.groups?.get("port")?.value?.toIntOrNull()
        if (valid(port)) {
            return PortError(port!!, pattern.source)
        }
    }
    val port = commandLine?.let { portFromCommand(it) } ?: return null
    return PORTLESS.firstOrNull { it.re.containsMatchIn(line) }?.let { PortError(port, it.source) }
}

// CSI sequences (colours, cursor moves), OSC sequences and two-byte escapes.
private val ANSI = Regex("""\u001b\[[0-?]*[ -/]*[@-~]|\u001b][^\u0007\u001b]*(?:\u0007|\u001b\\)|\u001b[@-Z\\-_]""")

/** Removes terminal escape sequences. */
fun stripAnsi(s: String): String = ANSI.replace(s, "")

/** Longest partial line kept between chunks; longer lines are cut. */
private const val MAX_PENDING = 4096

/**
 * Finds port errors in a process's output as it streams in: removes escape
 * codes, joins lines split across chunks, and reports each port once.
 */
class OutputScanner(private val commandLine: String? = null) {
    private var pending = ""
    private val seen = mutableSetOf<Int>()

    /** Feeds a chunk; returns errors for ports not reported before. */
    @Synchronized
    fun push(chunk: String): List<PortError> {
        val text = pending + stripAnsi(chunk)
        val cut = maxOf(text.lastIndexOf('\n'), text.lastIndexOf('\r'))
        pending = text.substring(cut + 1).takeLast(MAX_PENDING)
        return scan(text.substring(0, cut + 1))
    }

    /** Scans what's left once the output has ended. */
    @Synchronized
    fun end(): List<PortError> {
        val rest = pending
        pending = ""
        return scan(rest)
    }

    private fun scan(text: String): List<PortError> =
        text.split('\r', '\n').mapNotNull { matchPortError(it, commandLine) }.filter { seen.add(it.port) }
}
