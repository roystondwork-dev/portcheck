// The Network Tools panel's logic: validates what the webview sends and turns
// portcheck's JSON into table rows. Pure: no "vscode" import, so it is
// unit-tested in isolation; netPanel.ts only wires it to a webview.

import { deviceItems, ipItems, lookupQueries } from "./netModel";
import type {
  CheckData,
  DevicesData,
  DoctorData,
  LookupData,
  NetData,
  ScanData,
} from "./schema";

/** The panel's sections. */
export type Section = "ports" | "project" | "lookup" | "addresses" | "devices";

/** What the webview asks for. */
export type Request =
  | { type: "check"; host: string; ports: string }
  | { type: "lookup"; text: string }
  | { type: "addresses"; public: boolean }
  | { type: "devices"; probe: boolean }
  | { type: "copy"; text: string }
  | { type: "stop"; port: number }
  /** Re-check host and ports every few seconds until "unwatch". */
  | { type: "watch"; host: string; ports: string }
  | { type: "unwatch" }
  /** The workspace's .portcheck.yaml checks. */
  | { type: "project" };

/** How a row is marked: a green, red, amber or neutral pill. */
export type Tone = "ok" | "bad" | "warn" | "info";

export interface Row {
  cells: string[];
  tone?: Tone;
  /** The value the row's Copy button copies. */
  copy?: string;
  /** A local port the row's Stop button frees. */
  stopPort?: number;
  /** Its status changed since the previous check (while watching). */
  changed?: boolean;
}

/** What the panel shows for a section. */
export interface Result {
  type: "result";
  section: Section;
  /** Column headings. */
  columns: string[];
  rows: Row[];
  /** A one-line summary above the table. */
  summary?: string;
  /** Shown instead of the table when the request failed. */
  error?: string;
  /** The check runs again every few seconds. */
  watching?: boolean;
}

/** Ports per check: the same ceiling as a reasonable `portcheck scan -p`. */
export const MAX_PORTS = 1024;

/**
 * Reads a message from the webview. Anything malformed is dropped: the
 * webview is ours, but its messages are still untrusted input.
 */
export function parseRequest(msg: unknown): Request | undefined {
  if (typeof msg !== "object" || msg === null) {
    return undefined;
  }
  const m = msg as Record<string, unknown>;
  const str = (k: string) => (typeof m[k] === "string" ? m[k] : undefined);
  switch (m.type) {
    case "check":
    case "watch": {
      const host = str("host");
      const ports = str("ports");
      return host !== undefined && ports !== undefined
        ? { type: m.type, host, ports }
        : undefined;
    }
    case "unwatch":
      return { type: "unwatch" };
    case "project":
      return { type: "project" };
    case "lookup": {
      const text = str("text");
      return text === undefined ? undefined : { type: "lookup", text };
    }
    case "addresses":
      return { type: "addresses", public: m.public === true };
    case "devices":
      return { type: "devices", probe: m.probe === true };
    case "copy": {
      const text = str("text");
      return text === undefined || text === ""
        ? undefined
        : { type: "copy", text };
    }
    case "stop":
      return typeof m.port === "number" && validPort(m.port)
        ? { type: "stop", port: m.port }
        : undefined;
    default:
      return undefined;
  }
}

function validPort(n: number): boolean {
  return Number.isInteger(n) && n >= 1 && n <= 65535;
}

/**
 * Parses "3000", "80,443" or "8000-8100" into a list of ports, or returns an
 * error message for the user.
 */
export function parsePorts(spec: string): number[] | string {
  const parts = spec
    .split(/[\s,]+/)
    .map((p) => p.trim())
    .filter((p) => p !== "");
  if (parts.length === 0) {
    return "Enter a port, e.g. 3000, 80,443 or 8000-8100.";
  }
  const ports = new Set<number>();
  for (const part of parts) {
    const m = /^(\d+)(?:-(\d+))?$/.exec(part);
    if (m === null) {
      return `"${part}" is not a port or a range.`;
    }
    const from = Number(m[1]);
    const to = m[2] === undefined ? from : Number(m[2]);
    if (!validPort(from) || !validPort(to)) {
      return `Ports go from 1 to 65535; "${part}" is outside that.`;
    }
    if (to < from) {
      return `"${part}" goes backwards; write the lower port first.`;
    }
    if (ports.size + (to - from + 1) > MAX_PORTS) {
      return `That's more than ${MAX_PORTS} ports; check fewer at a time.`;
    }
    for (let p = from; p <= to; p++) {
      ports.add(p);
    }
  }
  return [...ports].sort((a, b) => a - b);
}

/**
 * Checks a host name or address for the user, or returns why it can't be
 * used. Ranges (CIDR) are left to the command line on purpose.
 */
export function parseHost(input: string): string | { error: string } {
  const host = input.trim();
  if (host === "") {
    return "localhost";
  }
  if (/[\s/]/.test(host) || host.startsWith("-")) {
    return { error: "Enter one host name or IP address, e.g. db.internal." };
  }
  return host;
}

/** True for names and addresses that mean this machine. */
export function isLocal(host: string): boolean {
  const h = host.toLowerCase().replace(/^\[|\]$/g, "");
  return (
    h === "localhost" ||
    h.endsWith(".localhost") ||
    h === "::1" ||
    h === "0.0.0.0" ||
    h === "::" ||
    /^127\.\d+\.\d+\.\d+$/.test(h)
  );
}

/** Compresses a port list for the command line: 80,443,8000-8002. */
export function portSpec(ports: number[]): string {
  const out: string[] = [];
  for (let i = 0; i < ports.length;) {
    let j = i;
    while (j + 1 < ports.length && ports[j + 1] === (ports[j] as number) + 1) {
      j++;
    }
    out.push(
      i === j ? String(ports[i]) : `${String(ports[i])}-${String(ports[j])}`,
    );
    i = j + 1;
  }
  return out.join(",");
}

/** Local ports: free, or who holds them, with a Stop button. */
export function checkResult(data: CheckData): Result {
  const rows: Row[] = [];
  let busy = 0;
  for (const s of data.ports) {
    const port = `${s.port}/${s.proto}`;
    if (s.status === "free") {
      rows.push({ cells: [port, "free", "Nothing is listening"], tone: "ok" });
      continue;
    }
    if (s.status === "denied") {
      rows.push({
        cells: [port, "unknown", "Not allowed to check this port"],
        tone: "warn",
      });
      continue;
    }
    busy++;
    if (s.holders.length === 0) {
      rows.push({
        cells: [port, "in use", "by a process hidden from your user"],
        tone: "bad",
      });
      continue;
    }
    for (const h of s.holders) {
      rows.push(holderRow(port, s.port, h));
    }
  }
  const total = data.ports.length;
  return {
    type: "result",
    section: "ports",
    columns: ["Port", "Status", "Details"],
    rows,
    summary:
      busy === 0
        ? `${plural(total, "port")} on this machine: all free.`
        : `${plural(total, "port")} on this machine: ${busy} in use.`,
  };
}

function holderRow(
  port: string,
  n: number,
  h: CheckData["ports"][number]["holders"][number],
): Row {
  if (h.container) {
    const name = h.container.name ?? h.container.image ?? "a container";
    return {
      cells: [
        port,
        "in use",
        `${h.container.runtime} container ${name}. Stop it with: ${h.container.stopHint}`,
      ],
      tone: "bad",
      copy: h.container.stopHint,
    };
  }
  if (h.process === undefined) {
    return {
      cells: [port, "in use", "by a process hidden from your user"],
      tone: "bad",
    };
  }
  const p = h.process;
  const where = p.project === undefined ? "" : ` · in ${p.project}`;
  return {
    cells: [port, "in use", `${p.name} (pid ${p.pid})${where}`],
    tone: "bad",
    copy: String(p.pid),
    stopPort: n,
  };
}

const SCAN_TONES: Record<ScanData["results"][number]["state"], Tone> = {
  open: "ok",
  closed: "bad",
  filtered: "warn",
  error: "bad",
};

const SCAN_DETAILS: Record<ScanData["results"][number]["state"], string> = {
  open: "Accepts connections",
  closed: "The host refused the connection",
  filtered: "No answer in time (often a firewall)",
  error: "Couldn't connect",
};

/** Another host's ports: open, closed, filtered. */
export function scanResult(data: ScanData): Result {
  const rows = data.results.map((r): Row => {
    const target =
      r.address !== "" && r.address !== r.host
        ? `${r.host} (${r.address})`
        : r.host;
    const service = r.service === undefined ? "" : ` · ${r.service}`;
    const latency =
      r.state === "open" && r.latencyMs !== undefined
        ? ` · ${r.latencyMs.toFixed(1)} ms`
        : "";
    return {
      cells: [
        `${r.port}`,
        r.state,
        `${r.error ?? SCAN_DETAILS[r.state]}${service}${latency}`,
        target,
      ],
      tone: SCAN_TONES[r.state],
    };
  });
  const s = data.summary;
  return {
    type: "result",
    section: "ports",
    columns: ["Port", "Status", "Details", "Host"],
    rows,
    summary: `${plural(s.ports, "port")} checked in ${s.durationMs} ms: ${s.open} open, ${s.closed} closed, ${s.filtered} filtered${s.errors > 0 ? `, ${s.errors} failed` : ""}.`,
  };
}

/** Domain and address lookups. */
export function lookupResult(data: LookupData): Result {
  const rows = data.results.flatMap((r): Row[] =>
    r.error !== undefined
      ? [{ cells: [r.query, "doesn't resolve", r.error], tone: "bad" }]
      : r.records.map((rec) => ({
          cells: [
            r.query,
            rec.note === undefined ? rec.type : `${rec.type} · ${rec.note}`,
            rec.value,
          ],
          copy: rec.value,
        })),
  );
  return {
    type: "result",
    section: "lookup",
    columns: ["Query", "Type", "Value"],
    rows,
  };
}

/** This machine's addresses, gateway and DNS servers. */
export function addressesResult(data: NetData): Result {
  const rows = ipItems(data).flatMap((i): Row[] => {
    if (i.copy === undefined) {
      return []; // the "Show My Public Address" action: a button here
    }
    // Every address ipItems lists has a description.
    const what = i.description as string;
    const row: Row = { cells: [i.label, what], copy: i.copy };
    if (what.startsWith("primary")) {
      row.tone = "info";
    }
    return [row];
  });
  return {
    type: "result",
    section: "addresses",
    columns: ["Address", "What it is"],
    rows,
    summary:
      data.public === undefined
        ? "Read locally; nothing was sent over the internet."
        : "The public address comes from ipify.org.",
  };
}

/** Devices seen on the local network. */
export function devicesResult(data: DevicesData): Result {
  const rows = deviceItems(data).flatMap((i): Row[] =>
    i.copy === undefined
      ? [] // the "Probe for More Devices" action: a button here
      : [
          {
            cells: [i.label, i.description ?? "", i.detail ?? ""],
            copy: i.copy,
          },
        ],
  );
  return {
    type: "result",
    section: "devices",
    columns: ["Address", "Device", "MAC · Interface"],
    rows,
    summary:
      data.probed === undefined
        ? `${plural(rows.length, "device")} your machine has talked to recently. Probe to find more.`
        : `${plural(rows.length, "device")} on ${data.probed}.`,
  };
}

/** A failed request, shown in place of the section's table. */
export function errorResult(section: Section, message: string): Result {
  return { type: "result", section, columns: [], rows: [], error: message };
}

/** Lookup queries from the input, or an error message. */
export function parseLookup(text: string): string[] | string {
  const q = lookupQueries(text);
  return q.length === 0 ? "Enter a domain name or an IP address." : q;
}

/** How often a watched check runs: often for this machine, gently for others. */
export function watchInterval(host: string): number {
  return isLocal(host) ? 2000 : 5000;
}

/**
 * A row's state for spotting changes: its status, and on this machine also
 * who holds the port. Another host's details carry the latency, which
 * changes on every check, so only the status counts there.
 */
function stateOf(row: Row, local: boolean): string {
  const status = row.cells[1] as string;
  return local ? `${status}\t${row.cells[2] as string}` : status;
}

/** Each port's state (keyed by the first cell), for the next markChanges. */
export function rowStates(rows: Row[], host: string): Map<string, string> {
  const local = isLocal(host);
  const states = new Map<string, string>();
  for (const r of rows) {
    const key = r.cells[0] as string;
    // A port with several holders: the first one stands for the port.
    if (!states.has(key)) {
      states.set(key, stateOf(r, local));
    }
  }
  return states;
}

/**
 * Marks the rows whose state changed since the previous check and describes
 * the changes, e.g. "3000/tcp is now free" or "db.internal 5432 is now open".
 * The first check of a watch (no previous states) changes nothing.
 */
export function markChanges(
  previous: ReadonlyMap<string, string> | undefined,
  result: Result,
  host: string,
): string[] {
  if (previous === undefined) {
    return [];
  }
  const local = isLocal(host);
  const where = local ? "" : `${host} `;
  const messages: string[] = [];
  const reported = new Set<string>();
  for (const row of result.rows) {
    const key = row.cells[0] as string;
    const before = previous.get(key);
    if (before === undefined || before === stateOf(row, local)) {
      continue;
    }
    row.changed = true;
    if (!reported.has(key)) {
      reported.add(key);
      const status = row.cells[1] as string;
      messages.push(
        before.startsWith(`${status}\t`)
          ? `${key} is now held by ${row.cells[2] as string}`
          : `${where}${key} is now ${status}`,
      );
    }
  }
  return messages;
}

const EXPECT_TEXT = { free: "should be free", open: "should be open" };

/** The checks of the workspace's .portcheck.yaml. */
export function projectResult(data: DoctorData): Result {
  const rows = data.checks.map((c): Row => {
    const where = c.host === undefined ? "" : ` on ${c.host}`;
    // An ok check needs no explanation beyond what it expects.
    const detail = [
      c.detail ?? (c.ok ? "" : c.status),
      c.ok ? "" : (c.fix ?? ""),
    ]
      .filter((x) => x !== "")
      .join(". ");
    const row: Row = {
      cells: [
        c.name,
        `${c.port}/${c.proto}${where}`,
        c.ok ? "ok" : "needs attention",
        detail === ""
          ? EXPECT_TEXT[c.expect]
          : `${EXPECT_TEXT[c.expect]}: ${detail}`,
      ],
      tone: c.ok ? "ok" : "bad",
    };
    // A local port that should be free but isn't can be freed from here.
    if (!c.ok && c.expect === "free" && c.host === undefined) {
      row.stopPort = c.port;
    }
    if (!c.ok && c.fix !== undefined) {
      row.copy = c.fix;
    }
    return row;
  });
  const bad = rows.filter((r) => r.tone === "bad").length;
  return {
    type: "result",
    section: "project",
    columns: ["Name", "Port", "Status", "Details"],
    rows,
    summary:
      bad === 0
        ? `Project ports: all ${rows.length} ok.`
        : `Project ports: ${rows.length - bad} ok, ${bad} need attention.`,
  };
}

/** The Project ports section when there's no file, or it can't be read. */
export function projectMissingResult(invalid: boolean): Result {
  return invalid
    ? errorResult(
        "project",
        ".portcheck.yaml has errors: see the Problems panel.",
      )
    : {
        type: "result",
        section: "project",
        columns: [],
        rows: [],
        summary:
          "No .portcheck.yaml in this workspace. Add one to list your project's ports and check them here.",
      };
}

function plural(n: number, word: string): string {
  return `${n} ${word}${n === 1 ? "" : "s"}`;
}
