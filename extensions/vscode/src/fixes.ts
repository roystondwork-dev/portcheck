// What to offer when a terminal says a port is in use. Pure logic: the
// terminal link handler turns these into a quick pick and runs the chosen one.
import type { PortRow } from "./model";

export type ShellKind = "posix" | "powershell" | "cmd" | "nushell";

/** The command that failed, when the terminal's shell integration saw it. */
export interface LastCommand {
  commandLine: string;
  /** False while it's still running (e.g. a dev server that moved to another port). */
  finished: boolean;
}

export type FixAction =
  | { kind: "stop"; row: PortRow; rerun?: string }
  | { kind: "runCommand"; command: string }
  | { kind: "rerun"; command: string }
  | { kind: "copyPort"; port: number }
  | { kind: "details" };

export interface Fix {
  label: string;
  description?: string;
  action: FixAction;
}

export interface FixInput {
  port: number;
  status: "free" | "in-use" | "denied";
  holders: PortRow[];
  /** A free port to suggest instead, if one was found. */
  freePort?: number;
  last?: LastCommand;
  shell: ShellKind;
  /** Command that stops a hidden owner with elevated privileges. */
  elevatedKill: string;
}

/** Lists the fixes for a "port in use" error, most useful first. */
export function planFixes(i: FixInput): Fix[] {
  const fixes: Fix[] = [];
  const rerun = i.last?.finished ? i.last.commandLine : undefined;

  if (i.status === "free") {
    fixes.push(
      rerun
        ? { label: `$(debug-restart) Port ${i.port} is free now: re-run`, description: rerun, action: { kind: "rerun", command: rerun } }
        : { label: `$(check) Port ${i.port} is free now`, action: { kind: "details" } },
    );
  }

  for (const row of i.holders) {
    if (row.container) {
      fixes.push({
        label: `$(package) Stop ${owner(row)}`,
        description: row.container.stopHint,
        action: { kind: "runCommand", command: row.container.stopHint },
      });
    } else if (row.pid === undefined) {
      fixes.push({
        label: "$(lock) Stop the hidden owner (needs elevated privileges)",
        description: i.elevatedKill,
        action: { kind: "runCommand", command: i.elevatedKill },
      });
    } else {
      const started = row.ancestry ? `started by ${row.ancestry}` : "";
      if (rerun) {
        fixes.push({
          label: `$(debug-restart) Stop ${owner(row)} and re-run`,
          description: rerun,
          action: { kind: "stop", row, rerun },
        });
      }
      fixes.push({
        label: `$(debug-stop) Stop ${owner(row)}`,
        ...(started ? { description: started } : {}),
        action: { kind: "stop", row },
      });
    }
  }

  if (i.freePort !== undefined) {
    if (rerun) {
      const command = commandOnPort(rerun, i.port, i.freePort, i.shell);
      fixes.push({ label: `$(arrow-right) Re-run on free port ${i.freePort}`, description: command, action: { kind: "rerun", command } });
    }
    fixes.push({ label: `$(copy) Copy free port ${i.freePort}`, action: { kind: "copyPort", port: i.freePort } });
  }
  fixes.push({ label: "$(info) Show details", action: { kind: "details" } });
  return fixes;
}

function owner(row: PortRow): string {
  return row.label.replace(/^:\d+\s+/, "") + (row.pid === undefined || row.container ? "" : ` (pid ${row.pid})`);
}

// Tools whose port is set with a flag, and the flag to add.
const PORT_FLAGS: [RegExp, (p: number) => string][] = [
  [/\b(?:vite|astro dev|nuxt dev|nuxi dev|ng serve|next (?:dev|start)|hugo server|jekyll serve|flask run|remix vite:dev)\b/, (p) => ` --port ${p}`],
  [/\b(?:rails (?:s|server)|bin\/rails (?:s|server)|puma|rackup)\b/, (p) => ` -p ${p}`],
  [/\b(?:manage\.py runserver|http\.server)\s*$/, (p) => ` ${p}`],
];

/**
 * Rewrites a command to use newPort instead of oldPort: by replacing the port
 * if the command names it (--port 3000, :3000, runserver 8000), by adding the
 * tool's port flag, or by setting PORT, which most Node frameworks read.
 */
export function commandOnPort(cmd: string, oldPort: number, newPort: number, shell: ShellKind): string {
  const named = new RegExp(`(?<![\\d.])${oldPort}(?!\\d)`, "g");
  if (named.test(cmd)) {
    return cmd.replace(named, String(newPort));
  }
  for (const [tool, flag] of PORT_FLAGS) {
    if (tool.test(cmd)) {
      return cmd + flag(newPort);
    }
  }
  switch (shell) {
    case "powershell":
      return `$env:PORT=${newPort}; ${cmd}`;
    case "cmd":
      return `set PORT=${newPort}&& ${cmd}`;
    case "nushell":
      return `with-env { PORT: "${newPort}" } { ${cmd} }`;
    default:
      return `PORT=${newPort} ${cmd}`;
  }
}

/** The kind of shell, from its executable path. */
export function shellKind(shellPath: string | undefined): ShellKind {
  const name = (shellPath ?? "").replace(/^.*[\\/]/, "").replace(/\.exe$/i, "").toLowerCase();
  if (name === "pwsh" || name === "powershell") {
    return "powershell";
  }
  if (name === "cmd") {
    return "cmd";
  }
  if (name === "nu") {
    return "nushell";
  }
  return "posix";
}
