// Project ports: runs `portcheck doctor` on the workspace's .portcheck.yaml and
// shows the result in the status bar, as problems on the file, and in the view.
import * as vscode from "vscode";
import { PortcheckError } from "./cli";
import { type Deps, stopPort } from "./commands";
import { checkProblems, configErrors, type Problem, projectPorts, summarize } from "./projectModel";
import type { DoctorData } from "./schema";

const CONFIG_GLOB = "**/.portcheck.{yaml,yml}";

export class ProjectPorts implements vscode.Disposable {
  /** The latest doctor result; undefined when there's no file or it's invalid. */
  data: DoctorData | undefined;
  /** The .portcheck.yaml in use, if any. */
  config: vscode.Uri | undefined;
  private readonly status: vscode.StatusBarItem;
  private readonly problems: vscode.DiagnosticCollection;
  private readonly disposables: vscode.Disposable[] = [];
  private inFlight: Promise<void> | undefined;

  constructor(
    private readonly d: Deps,
    private readonly onProject: (ports: ReadonlyMap<number, string>) => void,
  ) {
    this.status = vscode.window.createStatusBarItem("portcheck.project", vscode.StatusBarAlignment.Left, 50);
    this.status.name = "portcheck: project ports";
    this.status.command = "portcheck.doctor";
    this.problems = vscode.languages.createDiagnosticCollection("portcheck");
    const watcher = vscode.workspace.createFileSystemWatcher(CONFIG_GLOB);
    const recheck = () => void this.refresh();
    this.disposables.push(
      this.status,
      this.problems,
      watcher,
      watcher.onDidChange(recheck),
      watcher.onDidCreate(recheck),
      watcher.onDidDelete(recheck),
      vscode.workspace.onDidChangeWorkspaceFolders(recheck),
      vscode.commands.registerCommand("portcheck.doctor", () => this.pickFix()),
    );
  }

  /** Re-runs doctor. Concurrent calls share one run. */
  refresh(): Promise<void> {
    this.inFlight ??= this.check().finally(() => {
      this.inFlight = undefined;
    });
    return this.inFlight;
  }

  private async findConfig(): Promise<vscode.Uri | undefined> {
    for (const folder of vscode.workspace.workspaceFolders ?? []) {
      for (const name of [".portcheck.yaml", ".portcheck.yml"]) {
        const uri = vscode.Uri.joinPath(folder.uri, name);
        try {
          await vscode.workspace.fs.stat(uri);
          return uri;
        } catch {
          // not in this folder
        }
      }
    }
    return undefined;
  }

  private async check(): Promise<void> {
    const uri = await this.findConfig();
    if (!uri) {
      this.config = undefined;
      this.data = undefined;
      this.problems.clear();
      this.status.hide();
      this.onProject(new Map());
      return;
    }
    try {
      const data = await this.d.client().doctor(uri.fsPath);
      const text = new TextDecoder().decode(await vscode.workspace.fs.readFile(uri));
      const s = summarize(data);
      this.status.text = s.text;
      this.status.tooltip = new vscode.MarkdownString(s.tooltip, true);
      this.status.backgroundColor = s.ok ? undefined : new vscode.ThemeColor("statusBarItem.warningBackground");
      this.show(uri, checkProblems(text, data));
      this.config = uri;
      this.data = data;
      this.onProject(projectPorts(data));
    } catch (err) {
      const invalid = err instanceof PortcheckError && err.exitCode === 2;
      const msg = err instanceof Error ? err.message : String(err);
      this.d.log.error(`doctor: ${msg}`);
      this.status.text = "$(error) ports";
      this.status.tooltip = invalid ? "`.portcheck.yaml` is invalid: see Problems." : `portcheck doctor failed: ${msg}`;
      this.status.backgroundColor = new vscode.ThemeColor("statusBarItem.errorBackground");
      this.show(uri, invalid ? configErrors(err.detail) : []);
      this.config = uri;
      this.data = undefined;
      this.onProject(new Map());
    }
    this.status.show();
  }

  private show(uri: vscode.Uri, problems: Problem[]): void {
    this.problems.clear();
    this.problems.set(
      uri,
      problems.map((p) => {
        const diag = new vscode.Diagnostic(
          new vscode.Range(p.line, 0, p.line, Number.MAX_SAFE_INTEGER),
          p.message,
          p.severity === "error" ? vscode.DiagnosticSeverity.Error : vscode.DiagnosticSeverity.Warning,
        );
        diag.source = "portcheck";
        return diag;
      }),
    );
  }

  /** The status bar's click: pick a failed check to fix, or re-check. */
  async pickFix(): Promise<string | undefined> {
    await this.refresh();
    if (!this.data) {
      if (this.config) {
        await vscode.window.showTextDocument(this.config);
      } else {
        void this.d.ui().info("No .portcheck.yaml in this workspace. See https://roystondwork-dev.github.io/portcheck/guide/doctor/");
      }
      return undefined;
    }
    const failed = this.data.checks.filter((c) => !c.ok);
    if (failed.length === 0) {
      void this.d.ui().info(`All ${this.data.checks.length} project ports are ok.`);
      return "ok";
    }
    const pick = await this.d.ui().pick(
      failed.map((c) => ({ label: `${c.name}: port ${c.port}`, description: c.detail ?? c.status, check: c })),
      "Project ports that need attention",
    );
    if (!pick) {
      return undefined;
    }
    const c = pick.check;
    if (c.expect === "free" && c.status === "in-use") {
      await stopPort(this.d, c.port);
    } else {
      // An "open" port is closed: the fix is to start the service.
      void this.d.ui().info(`${c.name}: ${c.detail ?? c.status}.${c.fix ? ` Fix: ${c.fix}` : ""}`);
    }
    await this.refresh();
    return c.name;
  }

  dispose(): void {
    for (const d of this.disposables) {
      d.dispose();
    }
  }
}
