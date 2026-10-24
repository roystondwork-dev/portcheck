package io.github.roystondworkdev.portcheck

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

class MatcherTest {
    // Real error lines, as the tools print them; the same corpus as the VS Code
    // extension's matcher.unit.test.ts, plus Java servers. Add a line here
    // whenever a tool's message isn't recognised.
    private val corpus = listOf(
        Triple("Error: listen EADDRINUSE: address already in use :::3000", 3000, "Node.js"),
        Triple("Error: listen EADDRINUSE: address already in use 127.0.0.1:8080", 8080, "Node.js"),
        Triple("Error: listen EADDRINUSE :::3000", 3000, "Node.js"),
        Triple("  code: 'EADDRINUSE', errno: -48, syscall: 'listen', address: '::', port: 3000", 3000, "Node.js"),
        Triple("[nodemon] Error: listen EADDRINUSE: address already in use 0.0.0.0:4000", 4000, "Node.js"),
        Triple("listen tcp :8080: bind: address already in use", 8080, "Go"),
        Triple("2026/10/20 12:00:00 listen tcp 127.0.0.1:1313: bind: address already in use", 1313, "Go"),
        Triple("listen tcp4 0.0.0.0:5432: bind: address already in use", 5432, "Go"),
        Triple("listen tcp [::1]:9000: bind: address already in use", 9000, "Go"),
        Triple("listen tcp :8080: bind: Only one usage of each socket address (protocol/network address/port) is normally permitted.", 8080, "Go"),
        Triple("Error response from daemon: driver failed programming external connectivity on endpoint web: Bind for 0.0.0.0:8080 failed: port is already allocated", 8080, "Docker"),
        Triple("Address already in use - bind(2) for \"127.0.0.1\" port 3000 (Errno::EADDRINUSE)", 3000, "Ruby"),
        Triple("Address already in use - bind(2) for \"::1\" port 3000 (Errno::EADDRINUSE)", 3000, "Ruby"),
        Triple("jekyll 3.9.0 | Error:  Address already in use - bind(2) for 127.0.0.1:4000", 4000, "Ruby"),
        Triple("Web server failed to start. Port 8080 was already in use.", 8080, "Spring Boot"),
        Triple("System.IO.IOException: Failed to bind to address http://127.0.0.1:5000: address already in use.", 5000, ".NET"),
        Triple("ERROR:    [Errno 48] error while attempting to bind on address ('127.0.0.1', 8000): address already in use", 8000, "Python"),
        Triple("Port 5000 is in use by another program. Either identify and stop that program, or start the server with a different port.", 5000, "Flask"),
        Triple("[Tue Oct 20 12:00:00 2026] Failed to listen on localhost:8000 (reason: Address already in use)", 8000, "PHP"),
        Triple("nginx: [emerg] bind() to 0.0.0.0:80 failed (98: Address already in use)", 80, "nginx"),
        Triple("# Warning: Could not create server TCP listening socket *:6379: bind: Address already in use", 6379, "Redis"),
        Triple("HINT:  Is another postmaster already running on port 5432? If not, wait a few seconds and retry.", 5432, "PostgreSQL"),
        Triple("? Something is already running on port 3000. Would you like to run the app on another port instead? » (Y/n)", 3000, "Create React App"),
        Triple("error: Failed to start server. Is port 3000 in use?", 3000, "Bun"),
        Triple("error when starting dev server:\nError: Port 5173 is already in use", 5173, "dev server"),
        Triple("Port 4200 is already in use. Use '--port' to specify a different port.", 4200, "dev server"),
        Triple("Port 5173 is in use, trying another one...", 5173, "dev server"),
        Triple(" ⚠ Port 3000 is in use, trying 3001 instead.", 3000, "dev server"),
        Triple("OSError: [Errno 98] Address already in use: port 8888 is already in use", 8888, "dev server"),
        Triple("socket.error: address already in use 0.0.0.0:9229", 9229, "unknown"),
        Triple("SEVERE: Failed to initialize end point associated with ProtocolHandler [\"http-nio-8080\"]", 8080, "Tomcat"),
        Triple("Failed to start end point associated with ProtocolHandler [\"https-jsse-nio-8443\"]", 8443, "Tomcat"),
    )

    @Test
    fun recognisesRealErrors() {
        for ((line, port, source) in corpus) {
            assertEquals(line, PortError(port, source), matchPortError(line))
        }
    }

    @Test
    fun ignoresOtherLines() {
        for (line in listOf(
            "Server listening on http://localhost:3000",
            "Port 3000 is free.",
            "OSError: [Errno 48] Address already in use",
            "Error: That port is already in use.",
            "Error: listen EADDRINUSE",
            "port 70000 is already in use",
            "Bind for 0.0.0.0:0 failed: port is already allocated",
            "java.net.BindException: Address already in use",
            "",
        )) {
            assertNull(line, matchPortError(line))
        }
    }

    @Test
    fun takesTheFirstErrorOnALine() {
        assertEquals(1, matchPortError("listen tcp :1: bind: address already in use; listen tcp :2: bind: address already in use")?.port)
    }

    @Test
    fun errorsWithoutAPortUseTheCommand() {
        for ((line, cmd, port, source) in listOf(
            Quad("OSError: [Errno 48] Address already in use", "python3 -m http.server 3000", 3000, "Python"),
            Quad("OSError: [Errno 98] Address already in use", "python -m http.server", 8000, "Python"),
            Quad("OSError: [WinError 10048] [Errno 10048] Address already in use", "uvicorn app:app --port 8001", 8001, "Python"),
            Quad("Error: That port is already in use.", "python manage.py runserver", 8000, "Django"),
            Quad("Error: That port is already in use.", "python manage.py runserver 0.0.0.0:8080", 8080, "Django"),
            Quad("error: Uncaught (in promise) AddrInUse: Address already in use (os error 48)", "deno run -A main.ts --port=8800", 8800, "Deno"),
            Quad("Error: Os { code: 98, kind: AddrInUse, message: \"Address already in use\" }", "cargo run -- -p 7878", 7878, "Rust"),
            Quad("Address already in use (os error 48)", "./server --bind 127.0.0.1:9090", 9090, "unknown"),
            Quad("Caused by: java.net.BindException: Address already in use", "./gradlew bootRun", 8080, "Java"),
        )) {
            assertEquals(line, PortError(port, source), matchPortError(line, cmd))
        }
        assertNull(matchPortError("OSError: [Errno 48] Address already in use", "python app.py"))
        assertNull(matchPortError("all good", "python -m http.server 3000"))
    }

    @Test
    fun portFromCommand() {
        val cases = mapOf(
            "vite --port 4000" to 4000,
            "next dev -p 3001" to 3001,
            "next dev -p3002" to 3002,
            "hugo server --bind=0.0.0.0:1414" to 1414,
            "php -S localhost:8080" to 8080,
            "python3 -m http.server 9000 --bind 127.0.0.1" to 9000,
            "vite" to 5173,
            "npx ng serve" to 4200,
            "bin/rails server" to 3000,
            "flask run" to 5000,
            "astro dev" to 4321,
            "bundle exec jekyll serve" to 4000,
            "hugo server" to 1313,
            "mvn spring-boot:run" to 8080,
            "npm run dev" to null,
            "node server.js --workers 4" to null,
            "serve -p 99999" to null,
        )
        for ((cmd, port) in cases) {
            assertEquals(cmd, port, portFromCommand(cmd))
        }
    }

    @Test
    fun stripAnsiRemovesColoursMarksAndShortEscapes() {
        assertEquals("Error: listen ok", stripAnsi("\u001b[31mError\u001b[0m: \u001b]633;C\u0007listen\u001b]0;title\u001b\\ \u001bMok"))
    }

    @Test
    fun scannerJoinsChunksAndStripsColours() {
        val s = OutputScanner()
        assertEquals(emptyList<PortError>(), s.push("starting...\r\n\u001b[31mError: listen EADDRIN"))
        assertEquals(listOf(PortError(3000, "Node.js")), s.push("USE: address already in use :::3000\u001b[0m\r\n    at Server.listen"))
        assertEquals(emptyList<PortError>(), s.end())
    }

    @Test
    fun scannerReportsEachPortOnceAndAFinalLine() {
        val s = OutputScanner()
        assertEquals(1, s.push("listen tcp :8080: bind: address already in use\nlisten tcp :8080: bind: address already in use\n").size)
        assertEquals(emptyList<PortError>(), s.push("Port 5173 is already in use"))
        assertEquals(listOf(5173), s.end().map { it.port })
        assertEquals(emptyList<PortError>(), s.end())
    }

    @Test
    fun scannerCapsAnEndlessLine() {
        val s = OutputScanner()
        s.push("x".repeat(10_000))
        assertEquals(listOf(9), s.push(" listen tcp :9: bind: address already in use\n").map { it.port })
    }

    @Test
    fun scannerUsesItsCommand() {
        val s = OutputScanner("python3 -m http.server 3000")
        assertEquals(listOf(3000), s.push("OSError: [Errno 48] Address already in use\n").map { it.port })
    }

    private data class Quad(val line: String, val cmd: String, val port: Int, val source: String)
}
