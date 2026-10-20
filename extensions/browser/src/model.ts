// What serves a port, from portcheck's check output. Pure; unit-tested.
import type { CheckData, ContainerInfo, ListData, ProcessInfo, Socket } from "./schema";

export interface Holder {
  /** e.g. "node", "docker:web" */
  name: string;
  pid?: number;
  user?: string;
  cmdline?: string;
  /** When the process started (RFC 3339). */
  startTime?: string;
  /** "npm (4200) ← zsh (900)", or "" */
  ancestry: string;
  container?: ContainerInfo;
  /** The owner is hidden from this user (needs elevated privileges). */
  hidden: boolean;
}

export interface PortState {
  port: number;
  status: "free" | "in-use" | "denied";
  holders: Holder[];
}

export function ancestry(p: ProcessInfo | undefined): string {
  return (p?.ancestors ?? []).map((a) => `${a.name} (${a.pid})`).join(" ← ");
}

export function containerLabel(c: ContainerInfo): string {
  const base = `${c.runtime}:${c.name ?? c.image ?? c.id ?? "?"}`;
  return c.process === undefined ? base : `${base}/${c.process}`;
}

function holder(s: Socket): Holder {
  const h: Holder = {
    name: s.container ? containerLabel(s.container) : (s.process?.name ?? "?"),
    ancestry: ancestry(s.process),
    hidden: s.pid === undefined && s.container === undefined,
  };
  if (s.pid !== undefined) {
    h.pid = s.pid;
  }
  if (s.process?.user !== undefined) {
    h.user = s.process.user;
  }
  if (s.process?.cmdline !== undefined) {
    h.cmdline = s.process.cmdline;
  }
  if (s.process?.startTime !== undefined) {
    h.startTime = s.process.startTime;
  }
  if (s.container !== undefined) {
    h.container = s.container;
  }
  return h;
}

/** The state of one port; holders are unique per process or container. */
export function portState(data: CheckData, port: number): PortState {
  const p = data.ports.find((x) => x.port === port);
  const holders: Holder[] = [];
  for (const s of p?.holders ?? []) {
    const h = holder(s);
    if (!holders.some((x) => x.name === h.name && x.pid === h.pid)) {
      holders.push(h);
    }
  }
  return { port, status: p?.status ?? "free", holders };
}

export interface Badge {
  text: string;
  color: string;
  title: string;
}

const GREEN = "#2E7D32";
const RED = "#C62828";
const GREY = "#616161";

/** The toolbar badge for a local page. */
export function badge(label: string, s: PortState): Badge {
  if (s.status === "free") {
    return { text: "off", color: RED, title: `Nothing is listening on ${label}. Is your server running?` };
  }
  if (s.status === "denied") {
    return { text: "!", color: GREY, title: `${label}: the port can't be used by your user.` };
  }
  const h = s.holders[0];
  if (h === undefined || h.hidden) {
    return { text: "?", color: GREY, title: `${label} is served by a process hidden from your user.` };
  }
  const who = h.pid === undefined || h.container ? h.name : `${h.name} (pid ${h.pid})`;
  const started = h.ancestry === "" ? "" : `, started by ${h.ancestry}`;
  const more = s.holders.length > 1 ? ` and ${s.holders.length - 1} more` : "";
  return { text: short(h), color: GREEN, title: `${label} is served by ${who}${started}${more}.` };
}

function short(h: Holder): string {
  const name = h.container ? (h.container.name ?? h.container.image ?? "ctr") : h.name.replace(/\.exe$/i, "");
  return name.slice(0, 4);
}

export interface ListedPort {
  port: number;
  /** e.g. "node", "docker:web, java" */
  owners: string;
  url: string;
}

/** Listening TCP ports with their owners, for the popup's list. */
export function listedPorts(data: ListData): ListedPort[] {
  const byPort = new Map<number, Set<string>>();
  for (const s of data.sockets) {
    if (s.proto !== "tcp") {
      continue;
    }
    const owners = byPort.get(s.localPort) ?? new Set<string>();
    owners.add(holder(s).hidden ? "(hidden)" : holder(s).name);
    byPort.set(s.localPort, owners);
  }
  return [...byPort.entries()]
    .sort(([a], [b]) => a - b)
    .map(([port, owners]) => ({ port, owners: [...owners].join(", "), url: `http://localhost:${port}/` }));
}
