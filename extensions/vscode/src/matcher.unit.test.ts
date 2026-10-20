import { describe, expect, it } from "vitest";
import { matchPortError, OutputScanner, portFromCommand, stripAnsi } from "./matcher";

// Real error lines, as the tools print them. Add a line here whenever a tool's
// message isn't recognised.
const CORPUS: [string, number, string][] = [
  ["Error: listen EADDRINUSE: address already in use :::3000", 3000, "Node.js"],
  ["Error: listen EADDRINUSE: address already in use 127.0.0.1:8080", 8080, "Node.js"],
  ["Error: listen EADDRINUSE :::3000", 3000, "Node.js"],
  ["  code: 'EADDRINUSE', errno: -48, syscall: 'listen', address: '::', port: 3000", 3000, "Node.js"],
  ["[nodemon] Error: listen EADDRINUSE: address already in use 0.0.0.0:4000", 4000, "Node.js"],
  ["listen tcp :8080: bind: address already in use", 8080, "Go"],
  ["2026/10/20 12:00:00 listen tcp 127.0.0.1:1313: bind: address already in use", 1313, "Go"],
  ["listen tcp4 0.0.0.0:5432: bind: address already in use", 5432, "Go"],
  ["listen tcp [::1]:9000: bind: address already in use", 9000, "Go"],
  [
    "listen tcp :8080: bind: Only one usage of each socket address (protocol/network address/port) is normally permitted.",
    8080,
    "Go",
  ],
  [
    "Error response from daemon: driver failed programming external connectivity on endpoint web: Bind for 0.0.0.0:8080 failed: port is already allocated",
    8080,
    "Docker",
  ],
  ['Address already in use - bind(2) for "127.0.0.1" port 3000 (Errno::EADDRINUSE)', 3000, "Ruby"],
  ['Address already in use - bind(2) for "::1" port 3000 (Errno::EADDRINUSE)', 3000, "Ruby"],
  ["jekyll 3.9.0 | Error:  Address already in use - bind(2) for 127.0.0.1:4000", 4000, "Ruby"],
  ["Web server failed to start. Port 8080 was already in use.", 8080, "Spring Boot"],
  [
    "System.IO.IOException: Failed to bind to address http://127.0.0.1:5000: address already in use.",
    5000,
    ".NET",
  ],
  [
    "ERROR:    [Errno 48] error while attempting to bind on address ('127.0.0.1', 8000): address already in use",
    8000,
    "Python",
  ],
  ["Port 5000 is in use by another program. Either identify and stop that program, or start the server with a different port.", 5000, "Flask"],
  ["[Tue Oct 20 12:00:00 2026] Failed to listen on localhost:8000 (reason: Address already in use)", 8000, "PHP"],
  ["nginx: [emerg] bind() to 0.0.0.0:80 failed (98: Address already in use)", 80, "nginx"],
  ["# Warning: Could not create server TCP listening socket *:6379: bind: Address already in use", 6379, "Redis"],
  ["HINT:  Is another postmaster already running on port 5432? If not, wait a few seconds and retry.", 5432, "PostgreSQL"],
  ["? Something is already running on port 3000. Would you like to run the app on another port instead? » (Y/n)", 3000, "Create React App"],
  ["error: Failed to start server. Is port 3000 in use?", 3000, "Bun"],
  ["error when starting dev server:\nError: Port 5173 is already in use", 5173, "dev server"],
  ["Port 4200 is already in use. Use '--port' to specify a different port.", 4200, "dev server"],
  ["Port 5173 is in use, trying another one...", 5173, "dev server"],
  [" ⚠ Port 3000 is in use, trying 3001 instead.", 3000, "dev server"],
  ["OSError: [Errno 98] Address already in use: port 8888 is already in use", 8888, "dev server"],
  ["socket.error: address already in use 0.0.0.0:9229", 9229, "unknown"],
];

describe("matchPortError", () => {
  it.each(CORPUS)("%s", (line, port, source) => {
    const m = matchPortError(line);
    expect(m).toMatchObject({ port, source });
    expect(line.slice(m?.start, (m?.start ?? 0) + (m?.length ?? 0))).toContain(String(port));
  });

  it.each([
    "Server listening on http://localhost:3000",
    "Port 3000 is free.",
    "OSError: [Errno 48] Address already in use",
    "Error: That port is already in use.",
    "Error: listen EADDRINUSE",
    "port 70000 is already in use",
    "Bind for 0.0.0.0:0 failed: port is already allocated",
    "",
  ])("ignores %j", (line) => {
    expect(matchPortError(line)).toBeUndefined();
  });

  it("takes the first error on a line", () => {
    expect(matchPortError("listen tcp :1: bind: address already in use; listen tcp :2: bind: address already in use")?.port).toBe(1);
  });
});

describe("stripAnsi", () => {
  it("removes colours, OSC marks and short escapes", () => {
    expect(stripAnsi("\x1b[31mError\x1b[0m: \x1b]633;C\x07listen\x1b]0;title\x1b\\ \x1bMok")).toBe("Error: listen ok");
  });
});

describe("OutputScanner", () => {
  it("finds errors split across chunks and coloured", () => {
    const s = new OutputScanner();
    expect(s.push("starting...\r\n\x1b[31mError: listen EADDRIN")).toEqual([]);
    expect(s.push("USE: address already in use :::3000\x1b[0m\r\n    at Server.listen")).toMatchObject([{ port: 3000, source: "Node.js" }]);
    expect(s.end()).toEqual([]);
  });

  it("reports each port once, and a final line without newline", () => {
    const s = new OutputScanner();
    expect(s.push("listen tcp :8080: bind: address already in use\nlisten tcp :8080: bind: address already in use\n")).toHaveLength(1);
    expect(s.push("Port 5173 is already in use")).toEqual([]);
    expect(s.end()).toMatchObject([{ port: 5173 }]);
    expect(s.end()).toEqual([]);
  });

  it("caps an endless line", () => {
    const s = new OutputScanner();
    s.push("x".repeat(10_000));
    expect(s.push(" listen tcp :9: bind: address already in use\n")).toMatchObject([{ port: 9 }]);
  });
});

describe("errors without a port", () => {
  it.each([
    ["OSError: [Errno 48] Address already in use", "python3 -m http.server 3000", 3000, "Python"],
    ["OSError: [Errno 98] Address already in use", "python -m http.server", 8000, "Python"],
    ["OSError: [WinError 10048] [Errno 10048] Address already in use", "uvicorn app:app --port 8001", 8001, "Python"],
    ["Error: That port is already in use.", "python manage.py runserver", 8000, "Django"],
    ["Error: That port is already in use.", "python manage.py runserver 0.0.0.0:8080", 8080, "Django"],
    ["error: Uncaught (in promise) AddrInUse: Address already in use (os error 48)", "deno run -A main.ts --port=8800", 8800, "Deno"],
    ['Error: Os { code: 98, kind: AddrInUse, message: "Address already in use" }', "cargo run -- -p 7878", 7878, "Rust"],
    ["Address already in use (os error 48)", "./server --bind 127.0.0.1:9090", 9090, "unknown"],
  ])("%s (from %s)", (line, cmd, port, source) => {
    expect(matchPortError(line, cmd)).toMatchObject({ port, source, start: line.search(/\[Errno|That|AddrInUse|kind|Address/) });
  });

  it("needs a command that names or implies the port", () => {
    expect(matchPortError("OSError: [Errno 48] Address already in use")).toBeUndefined();
    expect(matchPortError("OSError: [Errno 48] Address already in use", "python app.py")).toBeUndefined();
    expect(matchPortError("all good", "python -m http.server 3000")).toBeUndefined();
  });

  it("the scanner uses its command", () => {
    const s = new OutputScanner("python3 -m http.server 3000");
    expect(s.push("OSError: [Errno 48] Address already in use\n")).toMatchObject([{ port: 3000 }]);
  });
});

describe("portFromCommand", () => {
  it.each([
    ["vite --port 4000", 4000],
    ["next dev -p 3001", 3001],
    ["next dev -p3002", 3002],
    ["hugo server --bind=0.0.0.0:1414", 1414],
    ["php -S localhost:8080", 8080],
    ["python3 -m http.server 9000 --bind 127.0.0.1", 9000],
    ["vite", 5173],
    ["npx ng serve", 4200],
    ["bin/rails server", 3000],
    ["flask run", 5000],
    ["astro dev", 4321],
    ["bundle exec jekyll serve", 4000],
    ["hugo server", 1313],
    ["npm run dev", undefined],
    ["node server.js --workers 4", undefined],
    ["serve -p 99999", undefined],
  ] as const)("%s → %s", (cmd, port) => {
    expect(portFromCommand(cmd)).toBe(port);
  });
});
