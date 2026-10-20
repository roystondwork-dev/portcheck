// Recognises "port already in use" errors printed by dev servers, databases
// and tools, so the terminal can turn them into links to a fix.

export interface PortError {
  port: number;
  /** Where the match is in the line, for the terminal link. */
  start: number;
  length: number;
  /** What printed it, e.g. "Node.js"; for the log and tooltip. */
  source: string;
}

interface Pattern {
  source: string;
  re: RegExp;
}

// Each pattern captures the port in the named group "port". Written against
// real output; the corpus is in matcher.unit.test.ts. Order matters: the
// first matching pattern wins.
const PATTERNS: Pattern[] = [
  // Error: listen EADDRINUSE: address already in use :::3000 (also 127.0.0.1:3000, older "listen EADDRINUSE :::3000")
  { source: "Node.js", re: /EADDRINUSE(?::? address already in use)?:? (?:\S*:)?(?<port>\d{1,5})\b/ },
  // The error object Node prints: code: 'EADDRINUSE', errno: -48, syscall: 'listen', address: '::', port: 3000
  { source: "Node.js", re: /code: 'EADDRINUSE'.*\bport: (?<port>\d{1,5})\b/ },
  // listen tcp :8080: bind: address already in use / Only one usage of each socket address … (Go on Windows)
  {
    source: "Go",
    re: /listen (?:tcp|udp)[46]? \S*?:(?<port>\d{1,5}): bind: (?:address already in use|Only one usage of each socket address)/i,
  },
  // Bind for 0.0.0.0:8080 failed: port is already allocated
  { source: "Docker", re: /Bind for \S*:(?<port>\d{1,5}) failed: port is already allocated/ },
  // Address already in use - bind(2) for "127.0.0.1" port 3000 (Errno::EADDRINUSE)
  { source: "Ruby", re: /Address already in use - bind\(2\) for "?[^"\s]*"? port (?<port>\d{1,5})/ },
  // Address already in use - bind(2) for 127.0.0.1:4000 (WEBrick, Jekyll)
  { source: "Ruby", re: /Address already in use - bind\(2\) for \S*:(?<port>\d{1,5})/ },
  // Web server failed to start. Port 8080 was already in use.
  { source: "Spring Boot", re: /Port (?<port>\d{1,5}) was already in use/ },
  // Failed to bind to address http://127.0.0.1:5000: address already in use.
  { source: ".NET", re: /Failed to bind to address \S*?:(?<port>\d{1,5})\/?:? address already in use/i },
  // [Errno 48] error while attempting to bind on address ('127.0.0.1', 8000): address already in use
  { source: "Python", re: /error while attempting to bind on address \('[^']*', (?<port>\d{1,5})\)/ },
  // Port 5000 is in use by another program. (Flask)
  { source: "Flask", re: /Port (?<port>\d{1,5}) is in use by another program/ },
  // Failed to listen on localhost:8000 (reason: Address already in use)
  { source: "PHP", re: /Failed to listen on \S*:(?<port>\d{1,5}) \(reason: Address already in use\)/ },
  // nginx: [emerg] bind() to 0.0.0.0:80 failed (98: Address already in use)
  { source: "nginx", re: /bind\(\) to \S*:(?<port>\d{1,5}) failed \(\d+: Address already in use\)/ },
  // Could not create server TCP listening socket *:6379: bind: Address already in use
  { source: "Redis", re: /listening socket \S*:(?<port>\d{1,5}): bind: Address already in use/ },
  // HINT:  Is another postmaster already running on port 5432?
  { source: "PostgreSQL", re: /another postmaster already running on port (?<port>\d{1,5})/ },
  // Something is already running on port 3000. (Create React App)
  { source: "Create React App", re: /Something is already running on port (?<port>\d{1,5})/ },
  // Failed to start server. Is port 3000 in use? (Bun)
  { source: "Bun", re: /Failed to start server\. Is port (?<port>\d{1,5}) in use\?/ },
  // Error: Port 5173 is already in use (Vite --strictPort); Port 4200 is already in use. (Angular)
  { source: "dev server", re: /\bPort (?<port>\d{1,5}) is already in use/i },
  // Port 5173 is in use, trying another one... (Vite); ⚠ Port 3000 is in use, trying 3001 instead. (Next.js)
  { source: "dev server", re: /\bPort (?<port>\d{1,5}) is in use, trying/i },
  // Generic: "address already in use … :3000" / "port 3000 … already in use"
  { source: "unknown", re: /address already in use\S*\s+\S*?:(?<port>\d{1,5})\b/i },
  { source: "unknown", re: /\bport (?<port>\d{1,5})\b[^.]*\balready in use/i },
];

// Errors that don't say which port: the port comes from the command that failed.
const PORTLESS: Pattern[] = [
  // OSError: [Errno 48] Address already in use (macOS 48, Linux 98, Windows 10048)
  { source: "Python", re: /\[Errno (?:48|98|10048)\] Address already in use/ },
  // Error: That port is already in use. (Django)
  { source: "Django", re: /That port is already in use/ },
  // error: Uncaught (in promise) AddrInUse: Address already in use (os error 48) (Deno)
  { source: "Deno", re: /AddrInUse: Address already in use/ },
  // Os { code: 98, kind: AddrInUse, message: "Address already in use" } (Rust)
  { source: "Rust", re: /kind: AddrInUse/ },
  { source: "unknown", re: /Address already in use \(os error \d+\)/i },
];

// Default ports of tools, for commands that don't name one.
const DEFAULT_PORTS: [RegExp, number][] = [
  [/\bmanage\.py runserver\b/, 8000],
  [/\bhttp\.server\b/, 8000],
  [/\bflask run\b/, 5000],
  [/\b(?:rails (?:s|server)|next (?:dev|start)|react-scripts start)\b/, 3000],
  [/\bvite\b/, 5173],
  [/\bng serve\b/, 4200],
  [/\bastro dev\b/, 4321],
  [/\bhugo server\b/, 1313],
  [/\bjekyll serve\b/, 4000],
];

/** The port a command will listen on, when it names one or the tool has a known default. */
export function portFromCommand(cmd: string): number | undefined {
  const named =
    /(?:--port[= ]|-p[= ]?|--bind[= ]\S*:|\s\S*:)(?<port>\d{2,5})\b/.exec(cmd) ??
    /\s(?<port>\d{2,5})(?=\s|$)/.exec(cmd);
  const port = Number(named?.groups?.port);
  if (port >= 1 && port <= 65535) {
    return port;
  }
  return DEFAULT_PORTS.find(([re]) => re.test(cmd))?.[1];
}

/**
 * Finds a "port in use" error in one line of terminal output. For errors that
 * don't name the port, commandLine (the command that printed it) supplies it.
 */
export function matchPortError(line: string, commandLine?: string): PortError | undefined {
  // Terminal lines can carry colour codes; links index into the visible text,
  // which VS Code gives us without them, so no stripping is needed here.
  for (const p of PATTERNS) {
    const m = p.re.exec(line);
    const port = Number(m?.groups?.port);
    if (m && port >= 1 && port <= 65535) {
      return { port, start: m.index, length: m[0].length, source: p.source };
    }
  }
  const port = commandLine === undefined ? undefined : portFromCommand(commandLine);
  if (port === undefined) {
    return undefined;
  }
  for (const p of PORTLESS) {
    const m = p.re.exec(line);
    if (m) {
      return { port, start: m.index, length: m[0].length, source: p.source };
    }
  }
  return undefined;
}

// CSI sequences (colours, cursor moves), OSC sequences (titles, shell
// integration marks) and two-byte escapes.
// eslint-disable-next-line no-control-regex -- matching terminal escape codes is the point
const ANSI = /\x1b\[[0-?]*[ -/]*[@-~]|\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)|\x1b[@-Z\\-_]/g;

/** Removes terminal escape sequences. */
export function stripAnsi(s: string): string {
  return s.replace(ANSI, "");
}

/** Longest partial line kept between chunks; longer lines are cut. */
const MAX_PENDING = 4096;

/**
 * Finds port errors in a command's raw output as it streams in: removes escape
 * codes, joins lines split across chunks, and reports each port once.
 */
export class OutputScanner {
  private pending = "";
  private readonly seen = new Set<number>();

  /** commandLine: the command whose output this is, for errors that don't name the port. */
  constructor(private readonly commandLine?: string) {}

  /** Feeds a chunk; returns errors for ports not reported before. */
  push(chunk: string): PortError[] {
    const text = this.pending + stripAnsi(chunk);
    const cut = Math.max(text.lastIndexOf("\n"), text.lastIndexOf("\r"));
    this.pending = text.slice(cut + 1).slice(-MAX_PENDING);
    return this.scan(text.slice(0, cut + 1));
  }

  /** Scans what's left once the output has ended. */
  end(): PortError[] {
    const rest = this.pending;
    this.pending = "";
    return this.scan(rest);
  }

  private scan(text: string): PortError[] {
    const found: PortError[] = [];
    for (const line of text.split(/[\r\n]+/)) {
      const m = matchPortError(line, this.commandLine);
      if (m && !this.seen.has(m.port)) {
        this.seen.add(m.port);
        found.push(m);
      }
    }
    return found;
  }
}
