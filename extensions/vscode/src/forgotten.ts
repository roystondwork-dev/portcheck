// Finds dev servers that look forgotten: processes running in a project for a
// long time. Pure: no "vscode" import, so it is unit-tested in isolation.

import type { ListData } from "./schema";

export interface Forgotten {
  pid: number;
  name: string;
  project: string;
  ports: number[];
  /** When the process started, in ms since the epoch. */
  started: number;
}

export const HOUR = 3_600_000;

/**
 * Processes with a project that have run at least afterMs, oldest first, one
 * per process. Containers and processes without a start time are left out;
 * portcheck only knows a process's project when it can read its working
 * directory, which in practice means your own processes.
 */
export function forgottenServers(
  data: ListData,
  nowMs: number,
  afterMs: number,
): Forgotten[] {
  const byPid = new Map<number, Forgotten>();
  for (const s of data.sockets) {
    const p = s.process;
    if (
      s.container !== undefined ||
      p?.project === undefined ||
      p.startTime === undefined
    ) {
      continue;
    }
    const started = Date.parse(p.startTime);
    if (Number.isNaN(started) || nowMs - started < afterMs) {
      continue;
    }
    const f = byPid.get(p.pid) ?? {
      pid: p.pid,
      name: p.name,
      project: p.project,
      ports: [],
      started,
    };
    if (!f.ports.includes(s.localPort)) {
      f.ports.push(s.localPort);
    }
    byPid.set(p.pid, f);
  }
  return [...byPid.values()]
    .map((f) => ({ ...f, ports: f.ports.sort((a, b) => a - b) }))
    .sort((a, b) => a.started - b.started);
}

/** "2 dev servers have been running for over a day: node in shop (:5173), …" */
export function forgottenMessage(list: Forgotten[], afterMs: number): string {
  const days = Math.floor(afterMs / (24 * HOUR));
  const span =
    days >= 2
      ? `over ${days} days`
      : days === 1
        ? "over a day"
        : `over ${Math.max(Math.floor(afterMs / HOUR), 1)} hours`;
  const names = list.map(
    (f) =>
      `${f.name} in ${f.project} (${f.ports.map((p) => `:${p}`).join(", ")})`,
  );
  const shown =
    names.length > 3
      ? [...names.slice(0, 3), `and ${names.length - 3} more`]
      : names;
  const subject =
    list.length === 1 ? "1 dev server has" : `${list.length} dev servers have`;
  return `${subject} been running for ${span}: ${shown.join(", ")}.`;
}
