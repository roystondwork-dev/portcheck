import * as vscode from "vscode";
import type { Deps } from "./commands";
import { forgottenMessage, forgottenServers, HOUR } from "./forgotten";

const FIRST_CHECK_MS = 20_000;
const CHECK_EVERY_MS = 30 * 60_000;

export interface ForgottenSettings {
  notify: boolean;
  afterHours: number;
}

/**
 * Every 30 minutes, looks for dev servers that have run in your projects for a
 * long time (default a day) and offers to stop them. Each server is mentioned
 * once per window; "Don't Show Again" turns the check off.
 */
export class ForgottenServers implements vscode.Disposable {
  private readonly notified = new Set<number>();
  private timer: NodeJS.Timeout | undefined;
  private first: NodeJS.Timeout | undefined;

  constructor(
    private readonly d: Deps,
    private readonly settings: () => ForgottenSettings,
    private readonly disable: () => PromiseLike<unknown>,
  ) {
    this.first = setTimeout(() => void this.check(), FIRST_CHECK_MS);
    this.timer = setInterval(() => void this.check(), CHECK_EVERY_MS);
  }

  /** Looks once and notifies about servers not mentioned before. Returns what it found. */
  async check(now = Date.now()): Promise<number[]> {
    const { notify, afterHours } = this.settings();
    if (!notify) {
      return [];
    }
    let data;
    try {
      data = await this.d.client().list();
    } catch (err) {
      this.d.log.warn(
        `forgotten servers: ${err instanceof Error ? err.message : String(err)}`,
      );
      return [];
    }
    const afterMs = Math.max(afterHours, 1) * HOUR;
    const all = forgottenServers(data, now, afterMs);
    const fresh = all.filter((f) => !this.notified.has(f.pid));
    if (fresh.length === 0) {
      return [];
    }
    for (const f of fresh) {
      this.notified.add(f.pid);
    }
    const stop = fresh.length === 1 ? "Stop It" : `Stop All ${fresh.length}`;
    const choice = await this.d
      .ui()
      .info(
        forgottenMessage(fresh, afterMs),
        stop,
        "Show Ports",
        "Don't Show Again",
      );
    if (choice === stop) {
      const results = await Promise.allSettled(
        fresh.map((f) => this.d.client().kill(f.pid)),
      );
      const failed = results.filter((r) => r.status === "rejected").length;
      if (failed > 0) {
        void this.d
          .ui()
          .warn(
            `portcheck: ${failed} of ${fresh.length} couldn't be stopped. See the portcheck output.`,
          );
      }
      void this.d.view.refresh();
    } else if (choice === "Show Ports") {
      await vscode.commands.executeCommand("portcheck.ports.focus");
    } else if (choice === "Don't Show Again") {
      await this.disable();
    }
    return fresh.map((f) => f.pid);
  }

  dispose(): void {
    clearTimeout(this.first);
    clearInterval(this.timer);
  }
}
