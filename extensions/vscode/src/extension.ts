import * as fs from "node:fs";
import * as vscode from "vscode";
import { PortcheckClient, resolveBinary } from "./cli";
import { registerCommands, type UI, vscodeUI } from "./commands";
import { nodeExec } from "./nodeExec";
import { PortsView } from "./portsView";

/** Returned from activate() for tests and other extensions. */
export interface PortcheckApi {
  client: () => PortcheckClient;
  binary: () => string;
  view: PortsView;
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
      exists: fs.existsSync,
    });

  let binary = findBinary();
  let client = new PortcheckClient(binary, nodeExec);
  log.info(`using ${binary}`);

  const view = new PortsView(() => client, log);
  const tree = vscode.window.createTreeView("portcheck.ports", { treeDataProvider: view });
  view.attach(tree, settings().get<number>("refreshInterval", 5));

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
    }),
  );

  let ui: UI = vscodeUI;
  const api: PortcheckApi = {
    client: () => client,
    binary: () => binary,
    view,
    setUI: (u) => {
      ui = u;
    },
  };
  registerCommands(ctx, { client: api.client, binary: api.binary, view, log, ui: () => ui });
  return api;
}

export function deactivate(): void {
  // Everything is disposed through ctx.subscriptions.
}
