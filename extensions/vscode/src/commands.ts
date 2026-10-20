import * as vscode from "vscode";
import type { PortcheckClient } from "./cli";
import { buildRows, containerLabel, type PortRow, validatePort } from "./model";
import type { PortsView } from "./portsView";

/** How commands talk to the user. Swappable so tests can script the answers. */
export interface UI {
  info(message: string, ...items: string[]): PromiseLike<string | undefined>;
  warn(message: string, ...items: string[]): PromiseLike<string | undefined>;
  error(message: string): PromiseLike<unknown>;
  /** A modal question; returns the chosen item or undefined if cancelled. */
  confirm(message: string, detail: string, ...items: string[]): PromiseLike<string | undefined>;
  input(options: vscode.InputBoxOptions): PromiseLike<string | undefined>;
  pick<T extends vscode.QuickPickItem>(items: T[], placeHolder: string): PromiseLike<T | undefined>;
}

export const vscodeUI: UI = {
  info: (m, ...items) => vscode.window.showInformationMessage(m, ...items),
  warn: (m, ...items) => vscode.window.showWarningMessage(m, ...items),
  error: (m) => vscode.window.showErrorMessage(m),
  confirm: (m, detail, ...items) => vscode.window.showWarningMessage(m, { modal: true, detail }, ...items),
  input: (o) => vscode.window.showInputBox(o),
  pick: (items, placeHolder) => vscode.window.showQuickPick(items, { placeHolder }),
};

export interface Deps {
  client: () => PortcheckClient;
  binary: () => string;
  view: PortsView;
  log: vscode.LogOutputChannel;
  ui: () => UI;
}

const DEFAULT_FREE_RANGE = "3000-9999";

export function registerCommands(ctx: vscode.ExtensionContext, d: Deps): void {
  const reg = (id: string, fn: (...args: never[]) => PromiseLike<unknown>) => {
    ctx.subscriptions.push(
      vscode.commands.registerCommand(id, async (...args: never[]) => {
        try {
          return await fn(...args);
        } catch (err) {
          const msg = err instanceof Error ? err.message : String(err);
          d.log.error(`${id}: ${msg}`);
          void d.ui().error(`portcheck: ${msg}`);
          return undefined;
        }
      }),
    );
  };

  reg("portcheck.refresh", () => d.view.refresh());
  reg("portcheck.showOutput", () => {
    d.log.show();
    return Promise.resolve();
  });
  reg("portcheck.copyPid", async (row: PortRow) => {
    if (row.pid !== undefined) {
      await vscode.env.clipboard.writeText(String(row.pid));
      void d.ui().info(`Copied PID ${row.pid}.`);
    }
  });
  reg("portcheck.openInBrowser", (row: PortRow) =>
    vscode.env.openExternal(vscode.Uri.parse(`http://localhost:${row.port}`)),
  );
  reg("portcheck.stop", (row: PortRow) => stopRow(d, row));
  reg("portcheck.checkPort", (port?: number) => checkPort(d, port));
  reg("portcheck.stopPort", (port?: number) => stopPort(d, port));
  reg("portcheck.findFreePort", (range?: string) => findFreePort(d, range));
}

async function askPort(d: Deps, prompt: string): Promise<number | undefined> {
  const v = await d.ui().input({ prompt, placeHolder: "3000", validateInput: validatePort });
  return v === undefined ? undefined : Number(v.trim());
}

/** Stops what holds a row's port: a process via portcheck kill, otherwise guidance. */
export async function stopRow(d: Deps, row: PortRow): Promise<string | undefined> {
  if (row.container) {
    const c = row.container;
    const pick = await d.ui().info(
      `Port ${row.port} belongs to ${containerLabel(c)}. Stop it there instead: ${c.stopHint}`,
      "Run in Terminal",
      "Copy Command",
    );
    return runOrCopy(pick, c.stopHint);
  }
  if (row.pid === undefined) {
    const cmd = elevatedKill(d, row.port);
    const pick = await d.ui().warn(
      `The owner of port ${row.port} is hidden from your user. Stopping it needs elevated privileges: ${cmd}`,
      "Run in Terminal",
      "Copy Command",
    );
    return runOrCopy(pick, cmd);
  }

  const detail = [row.ancestry ? `Started by ${row.ancestry}.` : "", "portcheck asks it to exit, then forces it if needed."]
    .filter((s) => s !== "")
    .join("\n");
  const choice = await d.ui().confirm(
    `Stop ${row.processName ?? "the process"} (pid ${row.pid}) on port ${row.port}?`,
    detail,
    "Stop",
    "Force Kill",
  );
  if (choice === undefined) {
    return undefined;
  }
  return killRow(d, row.pid, choice === "Force Kill");
}

/** Stops a process with portcheck kill (its guards apply) and reports the outcome. */
export async function killRow(d: Deps, pid: number, force = false): Promise<string | undefined> {
  const res = await d.client().kill(pid, force);
  const r = res.results[0];
  await d.view.refresh();
  if (r === undefined) {
    return undefined;
  }
  if (r.outcome === "failed") {
    void d.ui().error(`Could not stop ${r.name ?? "the process"} (pid ${r.pid}): ${r.error ?? "unknown error"}`);
  } else {
    void d.ui().info(`${r.name ?? "Process"} (pid ${r.pid}): ${r.outcome}.`);
  }
  return r.outcome;
}

/** Runs a command in a new "portcheck" terminal. */
export function runInTerminal(command: string): void {
  const t = vscode.window.createTerminal("portcheck");
  t.show();
  t.sendText(command);
}

/** The command that stops a hidden owner with elevated privileges. */
export function elevatedKill(d: Deps, port: number): string {
  return `${process.platform === "win32" ? "" : "sudo "}"${d.binary()}" kill ${port}`;
}

async function runOrCopy(pick: string | undefined, command: string): Promise<string | undefined> {
  if (pick === "Run in Terminal") {
    runInTerminal(command);
  } else if (pick === "Copy Command") {
    await vscode.env.clipboard.writeText(command);
  }
  return pick;
}

export async function rowsForPort(d: Deps, port: number): Promise<{ status: string; rows: PortRow[] }> {
  const res = await d.client().check(port);
  const st = res.ports[0];
  return { status: st?.status ?? "free", rows: buildRows({ sockets: st?.holders ?? [], partial: false }) };
}

async function checkPort(d: Deps, port?: number): Promise<string | undefined> {
  port ??= await askPort(d, "Port to check");
  if (port === undefined) {
    return undefined;
  }
  const { status, rows } = await rowsForPort(d, port);
  if (status === "free") {
    void d.ui().info(`Port ${port} is free.`);
    return status;
  }
  if (status === "denied") {
    void d.ui().warn(`Port ${port} can't be used by your user (permission denied).`);
    return status;
  }
  const who = rows.map((r) => r.label.replace(/^:\d+\s+/, "") + (r.pid === undefined ? "" : ` (pid ${r.pid})`)).join(", ");
  const pick = await d.ui().warn(`Port ${port} is in use${who ? ` by ${who}` : ""}.`, "Stop It");
  if (pick === "Stop It") {
    await stopPort(d, port);
  }
  return status;
}

export async function stopPort(d: Deps, port?: number): Promise<string | undefined> {
  port ??= await askPort(d, "Port to free");
  if (port === undefined) {
    return undefined;
  }
  const { status, rows } = await rowsForPort(d, port);
  if (rows.length === 0) {
    void d.ui().info(
      status === "free" ? `Nothing is listening on port ${port}.` : `Port ${port} is ${status}, but its owner isn't visible.`,
    );
    return undefined;
  }
  let row = rows[0];
  if (rows.length > 1) {
    const pick = await d.ui().pick(
      rows.map((r) => ({ label: r.label, description: r.description, row: r })),
      `Several owners share port ${port}; pick one`,
    );
    row = pick?.row;
  }
  return row ? stopRow(d, row) : undefined;
}

async function findFreePort(d: Deps, range?: string): Promise<number | undefined> {
  range ??= await d.ui().input({ prompt: "Range to search", value: DEFAULT_FREE_RANGE });
  if (range === undefined) {
    return undefined;
  }
  const res = await d.client().free(range.trim());
  const port = res.ports[0];
  if (port === undefined) {
    void d.ui().warn(`No free port in ${range}.`);
    return undefined;
  }
  const editor = vscode.window.activeTextEditor;
  const actions = editor ? ["Copy", "Insert"] : ["Copy"];
  const pick = await d.ui().info(`Port ${port} is free.`, ...actions);
  if (pick === "Copy") {
    await vscode.env.clipboard.writeText(String(port));
  } else if (pick === "Insert" && editor) {
    await editor.edit((e) => {
      e.replace(editor.selection, String(port));
    });
  }
  return port;
}
