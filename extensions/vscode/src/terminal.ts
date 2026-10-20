// The USP (ADR-0004): "port in use" errors in the integrated terminal become
// links, and with shell integration a notification, that open a fix menu.
import * as vscode from "vscode";
import { type Deps, elevatedKill, killRow, rowsForPort, runInTerminal } from "./commands";
import { type FixAction, type LastCommand, planFixes, shellKind } from "./fixes";
import { matchPortError, OutputScanner } from "./matcher";

export interface PortLink extends vscode.TerminalLink {
  port: number;
  terminal: vscode.Terminal;
}

/** How many ports after the busy one to search for a free one. */
const FREE_SEARCH = 100;

export class TerminalFixes implements vscode.TerminalLinkProvider<PortLink>, vscode.Disposable {
  private readonly last = new WeakMap<vscode.Terminal, LastCommand>();
  private readonly disposables: vscode.Disposable[] = [];

  constructor(
    private readonly d: Deps,
    private readonly notify: () => boolean,
  ) {
    this.disposables.push(
      vscode.window.registerTerminalLinkProvider(this),
      vscode.window.onDidStartTerminalShellExecution((e) => {
        const last: LastCommand = { commandLine: e.execution.commandLine.value, finished: false };
        this.last.set(e.terminal, last);
        void this.watch(e.terminal, e.execution, last);
      }),
      vscode.window.onDidEndTerminalShellExecution((e) => {
        const last = this.last.get(e.terminal);
        if (last) {
          last.finished = true;
        }
      }),
    );
  }

  provideTerminalLinks(context: vscode.TerminalLinkContext): PortLink[] {
    const m = matchPortError(context.line, this.last.get(context.terminal)?.commandLine);
    if (!m) {
      return [];
    }
    return [{ startIndex: m.start, length: m.length, tooltip: `Fix port ${m.port} (portcheck)`, port: m.port, terminal: context.terminal }];
  }

  async handleTerminalLink(link: PortLink): Promise<void> {
    await this.fix(link.port, link.terminal);
  }

  /** Reads a command's output as it runs and offers a fix for the first error per port. */
  private async watch(terminal: vscode.Terminal, execution: vscode.TerminalShellExecution, last: LastCommand): Promise<void> {
    if (!this.notify()) {
      return;
    }
    const scanner = new OutputScanner(last.commandLine);
    try {
      for await (const chunk of execution.read()) {
        for (const e of scanner.push(chunk)) {
          void this.offer(e.port, terminal, last);
        }
      }
      for (const e of scanner.end()) {
        void this.offer(e.port, terminal, last);
      }
    } catch (err) {
      this.d.log.warn(`reading terminal output: ${String(err)}`);
    }
  }

  private async offer(port: number, terminal: vscode.Terminal, last: LastCommand): Promise<void> {
    this.d.log.info(`port ${port} in use, seen in terminal output of: ${last.commandLine}`);
    const { rows } = await rowsForPort(this.d, port);
    const who = rows[0] ? ` by ${rows[0].label.replace(/^:\d+\s+/, "")}${rows[0].pid === undefined ? "" : ` (pid ${rows[0].pid})`}` : "";
    const pick = await this.d.ui().warn(`Port ${port} is already in use${who}.`, "Fix…");
    if (pick === "Fix…") {
      await this.fix(port, terminal);
    }
  }

  /**
   * Shows what can be done about a busy port and does the chosen fix. Returns
   * the kind of fix done, or undefined if cancelled (used by tests).
   */
  async fix(port: number, terminal: vscode.Terminal | undefined): Promise<FixAction["kind"] | undefined> {
    const d = this.d;
    try {
      const [held, free] = await Promise.all([
        rowsForPort(d, port),
        port < 65535 ? d.client().free(`${port + 1}-${Math.min(port + FREE_SEARCH, 65535)}`) : Promise.resolve({ ports: [] }),
      ]);
      const last = terminal ? this.last.get(terminal) : undefined;
      const freePort = free.ports[0];
      const fixes = planFixes({
        port,
        status: held.status as "free" | "in-use" | "denied",
        holders: held.rows,
        ...(freePort === undefined ? {} : { freePort }),
        ...(last === undefined ? {} : { last }),
        shell: shellKind(terminal ? shellPath(terminal) : vscode.env.shell),
        elevatedKill: elevatedKill(d, port),
      });
      const pick = await d.ui().pick(
        fixes.map((f) => ({ label: f.label, description: f.description ?? "", action: f.action })),
        `Port ${port} is in use: what should portcheck do?`,
      );
      if (!pick) {
        return undefined;
      }
      await this.run(pick.action, port, terminal);
      return pick.action.kind;
    } catch (err) {
      const msg = err instanceof Error ? err.message : String(err);
      d.log.error(`fix port ${port}: ${msg}`);
      void d.ui().error(`portcheck: ${msg}`);
      return undefined;
    }
  }

  private async run(a: FixAction, port: number, terminal: vscode.Terminal | undefined): Promise<void> {
    switch (a.kind) {
      case "stop": {
        const outcome = await killRow(this.d, a.row.pid as number);
        if (a.rerun && outcome !== undefined && outcome !== "failed") {
          rerun(terminal, a.rerun);
        }
        return;
      }
      case "runCommand":
        runInTerminal(a.command);
        return;
      case "rerun":
        rerun(terminal, a.command);
        return;
      case "copyPort":
        await vscode.env.clipboard.writeText(String(a.port));
        void this.d.ui().info(`Copied ${a.port}.`);
        return;
      case "details":
        await vscode.commands.executeCommand("workbench.view.extension.portcheck");
        await this.d.view.refresh();
        void this.d.ui().info(`Port ${port}: see Listening ports.`);
        return;
    }
  }

  dispose(): void {
    for (const d of this.disposables) {
      d.dispose();
    }
  }
}

function shellPath(t: vscode.Terminal): string | undefined {
  const o = t.creationOptions as vscode.TerminalOptions;
  return o.shellPath ?? vscode.env.shell;
}

/** Runs a command again in the terminal it came from (or a new one). */
function rerun(terminal: vscode.Terminal | undefined, command: string): void {
  if (!terminal) {
    runInTerminal(command);
    return;
  }
  terminal.show();
  if (terminal.shellIntegration) {
    terminal.shellIntegration.executeCommand(command);
  } else {
    terminal.sendText(command);
  }
}
