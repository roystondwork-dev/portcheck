// What the popup shows, as data. Pure; unit-tested. popup.ts turns it into DOM.
import type { HostProblem } from "./host";
import type { Holder, PortState } from "./model";

/** Icons the popup knows (inline SVG in popup.ts). */
export type Icon = "tree" | "terminal" | "user" | "clock" | "box" | "info" | "plus";

export interface Detail {
  icon: Icon;
  text: string;
  /** Render as code. */
  code?: boolean;
}

export interface Action {
  kind: "stop" | "copy";
  label: string;
  /** "danger" for Stop, "secondary" otherwise. */
  style: "danger" | "secondary";
  /** For "stop": the process; for "copy": the text. */
  pid?: number | undefined;
  text?: string;
}

export interface PortView {
  /** e.g. "localhost:3000" */
  address: string;
  status: "running" | "stopped" | "hidden" | "denied";
  /** The chip next to the address, e.g. "Running". */
  chip: string;
  /** e.g. "node · pid 4242", or a sentence for other states. */
  summary: string;
  details: Detail[];
  actions: Action[];
}

/** "just now", "12 min", "3 h", "2 d" since start, or undefined. */
export function uptime(start: string | undefined, now: number): string | undefined {
  if (start === undefined) {
    return undefined;
  }
  const t = Date.parse(start);
  if (Number.isNaN(t) || t > now) {
    return undefined;
  }
  const min = Math.floor((now - t) / 60_000);
  if (min < 1) {
    return "just now";
  }
  if (min < 60) {
    return `${min} min`;
  }
  const h = Math.floor(min / 60);
  return h < 48 ? `${h} h` : `${Math.floor(h / 24)} d`;
}

function details(h: Holder, others: Holder[], now: number): Detail[] {
  const d: Detail[] = [];
  if (h.container) {
    d.push({ icon: "box", text: h.container.image ? `${h.name} (${h.container.image})` : h.name });
  }
  if (h.ancestry !== "") {
    d.push({ icon: "tree", text: `Started by ${h.ancestry}` });
  }
  if (h.cmdline !== undefined) {
    d.push({ icon: "terminal", text: h.cmdline, code: true });
  }
  const meta = [h.user, uptime(h.startTime, now)].filter((x) => x !== undefined);
  if (meta.length > 0) {
    d.push({ icon: h.user === undefined ? "clock" : "user", text: meta.join(" · ") });
  }
  for (const o of others) {
    d.push({ icon: "plus", text: o.pid === undefined || o.container ? `Also ${o.name}` : `Also ${o.name} (pid ${o.pid})` });
  }
  return d;
}

/** Actions for a visible holder: a process (has a pid) or a container. */
function actions(h: Holder): Action[] {
  if (h.container) {
    // Never stop a container's proxy: that would affect every container.
    return [{ kind: "copy", label: "Copy stop command", style: "secondary", text: h.container.stopHint }];
  }
  return [
    { kind: "stop", label: "Stop process", style: "danger", pid: h.pid },
    { kind: "copy", label: "Copy PID", style: "secondary", text: String(h.pid) },
  ];
}

/** The popup's view of the current tab's port. */
export function portView(address: string, s: PortState, now: number): PortView {
  if (s.status === "free") {
    return {
      address,
      status: "stopped",
      chip: "Not running",
      summary: "Nothing is listening on this port.",
      details: [{ icon: "info", text: "Start your dev server, or check the port in its configuration." }],
      actions: [],
    };
  }
  if (s.status === "denied") {
    return { address, status: "denied", chip: "No access", summary: "Your user can't use this port.", details: [], actions: [] };
  }
  const [first, ...rest] = s.holders;
  if (first === undefined || first.hidden) {
    return {
      address,
      status: "hidden",
      chip: "Hidden",
      summary: "Served by a process of another user.",
      details: [{ icon: "info", text: "Run portcheck with elevated privileges to see and stop it." }],
      actions: [{ kind: "copy", label: "Copy command", style: "secondary", text: `sudo portcheck kill ${s.port}` }],
    };
  }
  const summary = first.pid === undefined || first.container ? first.name : `${first.name} · pid ${first.pid}`;
  return { address, status: "running", chip: "Running", summary, details: details(first, rest, now), actions: actions(first) };
}

export interface SetupHelp {
  headline: string;
  intro: string;
  steps: { text: string; command?: string }[];
}

/** The OS, as chrome.runtime.getPlatformInfo() names it ("mac", "win", "linux", …). */
export type Platform = string;

/** The one-line installer: installs portcheck and connects it to the browsers. */
export function installCommand(platform: Platform): { shell: string; command: string } {
  if (platform === "win") {
    return { shell: "PowerShell", command: "irm https://roystondwork-dev.github.io/portcheck/install.ps1 | iex" };
  }
  return { shell: "Terminal", command: "curl -fsSL https://roystondwork-dev.github.io/portcheck/install.sh | sh" };
}

/**
 * What to tell the user when portcheck can't be reached. extensionId is this
 * extension's ID: if portcheck doesn't know it yet (a store build), a second
 * step names it.
 */
export function setupHelp(problem: HostProblem, message: string, extensionId: string, knownIds: readonly string[], platform: Platform): SetupHelp {
  if (problem === "failed") {
    return { headline: "portcheck reported a problem", intro: message, steps: [] };
  }
  const install = installCommand(platform);
  const steps: SetupHelp["steps"] = [{ text: `Run this in ${install.shell}:`, command: install.command }];
  if (!knownIds.includes(extensionId)) {
    steps.push({ text: "Then allow this extension:", command: `portcheck browser install --extension-id ${extensionId}` });
  }
  if (problem === "forbidden") {
    return { headline: "Reconnect portcheck", intro: "portcheck is installed but doesn't allow this extension yet. Update it:", steps };
  }
  return {
    headline: "Connect portcheck",
    intro: "To see which process serves a port and stop it, the extension asks the free portcheck app. One command installs and connects it:",
    steps,
  };
}

export interface PortRow {
  port: number;
  owners: string;
  url: string;
  /** The port of the current tab. */
  current: boolean;
}

/** Ports matching a filter ("30", "node"), the current tab's port first. */
export function filterPorts(rows: { port: number; owners: string; url: string }[], filter: string, currentPort: number | undefined): PortRow[] {
  const q = filter.trim().toLowerCase();
  return rows
    .filter((r) => q === "" || String(r.port).includes(q) || r.owners.toLowerCase().includes(q))
    .map((r) => ({ ...r, current: r.port === currentPort }))
    .sort((a, b) => Number(b.current) - Number(a.current));
}
