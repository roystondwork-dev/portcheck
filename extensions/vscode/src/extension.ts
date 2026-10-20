import * as fs from "node:fs";
import * as vscode from "vscode";
import { PortcheckClient, resolveBinary } from "./cli";
import { registerCommands, type UI, vscodeUI } from "./commands";
import { ProjectPorts } from "./project";
import { TerminalFixes } from "./terminal";
import { validatePort } from "./model";
import { nodeExec } from "./nodeExec";
import { PortsView } from "./portsView";

/** Returned from activate() for tests and other extensions. */
export interface PortcheckApi {
  client: () => PortcheckClient;
  binary: () => string;
  view: PortsView;
  terminal: TerminalFixes;
  project: ProjectPorts;
  /** Replaces how the extension asks the user things (tests script the answers). */
  setUI: (ui: UI) => void;
}

export function activate(ctx: vscode.ExtensionContext): PortcheckApi {
  const log = vscode.window.createOutputChannel("portcheck", { log: true });
  const settings = () => vscode.workspace.getConfiguration("portcheck");
  const findBinary = () =>
    resolveBinary({
      configured: settings().get<string>("path", ""),
      extensionPath: ctx.extensionPath,
      platform: process.platform,
      arch: process.arch,
      exists: fs.existsSync,
    });

  let binary = findBinary();
  let client = new PortcheckClient(binary, nodeExec);
  log.info(`using ${binary}`);

  const view = new PortsView(() => client, log);
  const tree = vscode.window.createTreeView("portcheck.ports", { treeDataProvider: view });
  view.attach(tree, settings().get<number>("refreshInterval", 5));
  view.setFilter(settings().get<boolean>("showUdp", false));

  ctx.subscriptions.push(
    log,
    view,
    tree,
    vscode.workspace.onDidChangeConfiguration((e) => {
      if (e.affectsConfiguration("portcheck.path")) {
        binary = findBinary();
        client = new PortcheckClient(binary, nodeExec);
        log.info(`using ${binary}`);
        void view.refresh();
      }
      if (e.affectsConfiguration("portcheck.refreshInterval")) {
        view.setInterval(settings().get<number>("refreshInterval", 5));
      }
      if (e.affectsConfiguration("portcheck.showUdp")) {
        view.setFilter(settings().get<boolean>("showUdp", false));
      }
      if (e.affectsConfiguration("portcheck.path")) {
        void project.refresh();
      }
    }),
  );

  let ui: UI = vscodeUI;
  const deps = { client: () => client, binary: () => binary, view, log, ui: () => ui };
  registerCommands(ctx, deps);
  const terminal = new TerminalFixes(deps, () => settings().get<boolean>("terminal.notify", true));
  const project = new ProjectPorts(deps, (ports) => {
    view.setFilter(settings().get<boolean>("showUdp", false), ports);
  });
  ctx.subscriptions.push(
    terminal,
    project,
    vscode.commands.registerCommand("portcheck.fixPort", async (port?: number) => {
      const p =
        port ?? Number(await ui.input({ prompt: "Port to fix", placeHolder: "3000", validateInput: validatePort }));
      return Number.isInteger(p) && p > 0 ? terminal.fix(p, vscode.window.activeTerminal) : undefined;
    }),
    vscode.commands.registerCommand("portcheck.toggleUdp", () =>
      settings().update("showUdp", !settings().get<boolean>("showUdp", false), vscode.ConfigurationTarget.Global),
    ),
  );
  void project.refresh();

  return {
    client: () => client,
    binary: () => binary,
    view,
    terminal,
    project,
    setUI: (u) => {
      ui = u;
    },
  };
}

export function deactivate(): void {
  // Everything is disposed through ctx.subscriptions.
}
