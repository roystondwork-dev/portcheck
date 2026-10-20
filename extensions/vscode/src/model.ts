// Turns portcheck's socket list into rows for the Listening ports view. Pure:
// no "vscode" import, so it is unit-tested in isolation.

import type { ContainerInfo, ListData, ProcessInfo, Socket } from "./schema";

export type OwnerKind = "process" | "container" | "hidden";

export interface PortRow {
  /** Stable identity across refreshes. */
  id: string;
  port: number;
  proto: "tcp" | "udp";
  kind: OwnerKind;
  /** e.g. ":3000  node" */
  label: string;
  /** e.g. "tcp · 127.0.0.1, ::1 · pid 4242" */
  description: string;
  /** Markdown shown on hover. */
  tooltip: string;
  /** VS Code contextValue: "port.<kind>.<proto>", used by menus. */
  contextValue: string;
  pid?: number;
  processName?: string;
  ancestry?: string;
  container?: ContainerInfo;
}

/** "npm (4200) ← zsh (900)", or "" when unknown. */
export function ancestry(p: ProcessInfo | undefined): string {
  return (p?.ancestors ?? []).map((a) => `${a.name} (${a.pid})`).join(" ← ");
}

export function containerLabel(c: ContainerInfo): string {
  const base = `${c.runtime}:${c.name ?? c.image ?? c.id ?? "?"}`;
  return c.process === undefined ? base : `${base}/${c.process}`;
}

function kindOf(s: Socket): OwnerKind {
  if (s.container !== undefined) {
    return "container";
  }
  return s.pid === undefined ? "hidden" : "process";
}

/**
 * One row per (port, protocol, owner): the IPv4 and IPv6 listeners of the same
 * process are merged, and their addresses listed together.
 */
export function buildRows(data: ListData): PortRow[] {
  const groups = new Map<string, Socket[]>();
  for (const s of data.sockets) {
    const owner = s.container ? `c:${containerLabel(s.container)}` : `p:${s.pid ?? 0}`;
    const id = `${s.proto}:${s.localPort}:${owner}`;
    const g = groups.get(id);
    if (g) {
      g.push(s);
    } else {
      groups.set(id, [s]);
    }
  }

  const rows: PortRow[] = [];
  for (const [id, socks] of groups) {
    const s = socks[0] as Socket; // groups are never empty
    const kind = kindOf(s);
    const addresses = [...new Set(socks.map((x) => x.localAddress))].sort();
    const name =
      kind === "container"
        ? containerLabel(s.container as ContainerInfo)
        : kind === "hidden"
          ? "(owner hidden)"
          : (s.process?.name ?? "?");

    const desc = [s.proto, addresses.join(", ")];
    if (s.pid !== undefined) {
      desc.push(`pid ${s.pid}`);
    }
    const row: PortRow = {
      id,
      port: s.localPort,
      proto: s.proto,
      kind,
      label: `:${s.localPort}  ${name}`,
      description: desc.join(" · "),
      tooltip: tooltip(s, addresses),
      contextValue: `port.${kind}.${s.proto}`,
    };
    if (s.pid !== undefined) {
      row.pid = s.pid;
    }
    if (s.process !== undefined) {
      row.processName = s.process.name;
      const a = ancestry(s.process);
      if (a !== "") {
        row.ancestry = a;
      }
    }
    if (s.container !== undefined) {
      row.container = s.container;
    }
    rows.push(row);
  }
  return rows.sort((a, b) => a.port - b.port || a.proto.localeCompare(b.proto) || a.label.localeCompare(b.label));
}

function tooltip(s: Socket, addresses: string[]): string {
  const lines = [`**Port ${s.localPort}/${s.proto}** on ${addresses.map((a) => `\`${a}\``).join(", ")}`];
  if (s.container) {
    const c = s.container;
    lines.push(`Container: **${containerLabel(c)}**${c.image ? ` (${c.image})` : ""}`);
    lines.push(`Stop with: \`${c.stopHint}\``);
  }
  if (s.process) {
    const p = s.process;
    lines.push(`Process: **${p.name}** (pid ${p.pid})${p.user ? ` · user ${p.user}` : ""}`);
    const a = ancestry(p);
    if (a !== "") {
      lines.push(`Started by: ${a}`);
    }
    if (p.cmdline) {
      lines.push(`Command: \`${p.cmdline}\``);
    }
  } else if (!s.container) {
    lines.push("The owner is hidden from your user. Run portcheck with elevated privileges to see it.");
  }
  return lines.join("\n\n");
}

/** Validates a port typed by the user; returns an error message or undefined. */
export function validatePort(value: string): string | undefined {
  const n = Number(value.trim());
  return Number.isInteger(n) && n >= 1 && n <= 65535 ? undefined : "Enter a port between 1 and 65535";
}
