import { type ChildProcess, spawn } from "node:child_process";
import * as net from "node:net";
import * as vscode from "vscode";
import type { UI } from "../../commands";
import type { PortcheckApi } from "../../extension";
import type { PortRow } from "../../model";

/** Records messages and returns scripted answers in order. */
export class ScriptedUI implements UI {
  readonly messages: string[] = [];
  /**
   * answers: replies to info/warn/confirm/input, in order. picks: for each
   * quick pick, a fragment of the label to choose (undefined cancels); when
   * none is left, the first item is chosen.
   */
  constructor(
    private readonly answers: (string | undefined)[] = [],
    private readonly picks: (string | undefined)[] = [],
  ) {}
  private answer(kind: string, message: string): Promise<string | undefined> {
    this.messages.push(`${kind}: ${message}`);
    return Promise.resolve(this.answers.shift());
  }
  info(m: string): Promise<string | undefined> {
    return this.answer("info", m);
  }
  warn(m: string): Promise<string | undefined> {
    return this.answer("warn", m);
  }
  error(m: string): Promise<unknown> {
    return this.answer("error", m);
  }
  confirm(m: string, detail: string): Promise<string | undefined> {
    return this.answer("confirm", `${m} | ${detail}`);
  }
  input(o: vscode.InputBoxOptions): Promise<string | undefined> {
    return this.answer("input", o.prompt ?? "");
  }
  pick<T extends vscode.QuickPickItem>(items: T[]): Promise<T | undefined> {
    this.messages.push(`pick: ${items.map((i) => `${i.label} [${i.description ?? ""}]`).join(", ")}`);
    if (this.picks.length === 0) {
      return Promise.resolve(items[0]);
    }
    const want = this.picks.shift();
    return Promise.resolve(want === undefined ? undefined : items.find((i) => i.label.includes(want)));
  }
  has(fragment: string): boolean {
    return this.messages.some((m) => m.includes(fragment));
  }
}

export function listen(): Promise<net.Server> {
  return new Promise((resolve) => {
    const s = net.createServer().listen(0, "127.0.0.1", () => {
      resolve(s);
    });
  });
}

export function portOf(s: net.Server): number {
  return (s.address() as net.AddressInfo).port;
}

export function close(s: net.Server): Promise<void> {
  return new Promise((resolve) => s.close(() => {
    resolve();
  }));
}

/** Starts a separate node process holding a TCP port; resolves with it and the port. */
export function spawnHolder(): Promise<{ child: ChildProcess; port: number }> {
  const child = spawn(process.execPath, [
    "-e",
    "const s=require('net').createServer().listen(0,'127.0.0.1',()=>console.log(s.address().port));setInterval(()=>{},1e6)",
  ], { env: { ...process.env, ELECTRON_RUN_AS_NODE: "1" } });
  return new Promise((resolve, reject) => {
    child.stdout.once("data", (d: Buffer) => {
      resolve({ child, port: Number(d.toString().trim()) });
    });
    child.once("error", reject);
  });
}

export async function rowFor(api: PortcheckApi, port: number, pid?: number): Promise<PortRow> {
  for (let i = 0; i < 20; i++) {
    await api.view.refresh();
    const row = api.view.rows.find((r) => r.port === port && (pid === undefined || r.pid === pid));
    if (row) {
      return row;
    }
    await new Promise((r) => setTimeout(r, 250));
  }
  throw new Error(`no row for port ${port}`);
}

export function waitFor(cond: () => boolean, what: string, ms = 15_000): Promise<void> {
  const until = Date.now() + ms;
  return new Promise((resolve, reject) => {
    const tick = () => {
      if (cond()) {
        resolve();
      } else if (Date.now() > until) {
        reject(new Error(`timed out waiting for ${what}`));
      } else {
        setTimeout(tick, 100);
      }
    };
    tick();
  });
}

export async function activateExtension(): Promise<PortcheckApi> {
  const ext = vscode.extensions.getExtension<PortcheckApi>("roystondwork-dev.portcheck");
  if (!ext) {
    throw new Error("extension not found");
  }
  return ext.activate();
}
