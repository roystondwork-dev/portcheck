import * as vscode from "vscode";
import type { PortcheckClient } from "./cli";
import { buildRows, type OwnerKind, type PortRow, viewRows } from "./model";

const ICONS: Record<OwnerKind, string> = { process: "server-process", container: "package", hidden: "lock" };

/** The "Listening ports" tree. Refreshes on a timer only while visible. */
export class PortsView implements vscode.TreeDataProvider<PortRow>, vscode.Disposable {
  private readonly changed = new vscode.EventEmitter<undefined>();
  readonly onDidChangeTreeData = this.changed.event;

  /** The rows shown (after filtering), project ports first. */
  rows: PortRow[] = [];
  private all: PortRow[] = [];
  private showUdp = false;
  private project: ReadonlyMap<number, string> = new Map();
  private tree: vscode.TreeView<PortRow> | undefined;
  private timer: NodeJS.Timeout | undefined;
  private intervalSeconds = 0;
  private inFlight: Promise<void> | undefined;

  constructor(
    private readonly client: () => PortcheckClient,
    private readonly log: vscode.LogOutputChannel,
  ) {}

  attach(tree: vscode.TreeView<PortRow>, intervalSeconds: number): void {
    this.tree = tree;
    tree.onDidChangeVisibility((e) => {
      this.schedule();
      if (e.visible) {
        void this.refresh();
      }
    });
    this.setInterval(intervalSeconds);
    if (tree.visible) {
      void this.refresh();
    }
  }

  setInterval(seconds: number): void {
    this.intervalSeconds = seconds;
    this.schedule();
  }

  private schedule(): void {
    if (this.timer) {
      clearInterval(this.timer);
      this.timer = undefined;
    }
    if (this.intervalSeconds > 0 && this.tree?.visible) {
      this.timer = setInterval(() => void this.refresh(), this.intervalSeconds * 1000);
    }
  }

  /** Reloads the rows. Concurrent calls share one portcheck run. */
  refresh(): Promise<void> {
    this.inFlight ??= this.load().finally(() => {
      this.inFlight = undefined;
    });
    return this.inFlight;
  }

  private async load(): Promise<void> {
    try {
      const data = await this.client().list();
      this.all = buildRows(data);
      this.rows = viewRows(this.all, { showUdp: this.showUdp, project: this.project });
      this.setMessage(
        this.rows.length === 0
          ? "Nothing is listening."
          : data.partial
            ? "Some owners are hidden from your user (shown with a lock)."
            : undefined,
      );
    } catch (err) {
      const msg = err instanceof Error ? err.message : String(err);
      this.log.error(`list failed: ${msg}`);
      this.all = [];
      this.rows = [];
      this.setMessage(`portcheck failed: ${msg}`);
    }
    this.changed.fire(undefined);
  }

  /** Changes what's shown without running portcheck again. */
  setFilter(showUdp: boolean, project: ReadonlyMap<number, string> = this.project): void {
    this.showUdp = showUdp;
    this.project = project;
    this.rows = viewRows(this.all, { showUdp, project });
    this.changed.fire(undefined);
  }

  private setMessage(message: string | undefined): void {
    if (this.tree) {
      this.tree.message = message ?? ""; // empty hides the message
    }
  }

  getChildren(): PortRow[] {
    return this.rows;
  }

  getTreeItem(row: PortRow): vscode.TreeItem {
    const item = new vscode.TreeItem(row.label, vscode.TreeItemCollapsibleState.None);
    item.id = row.id;
    item.description = row.description;
    item.tooltip = new vscode.MarkdownString(row.tooltip);
    item.iconPath = new vscode.ThemeIcon(row.project === undefined ? ICONS[row.kind] : "star-full");
    item.contextValue = row.contextValue;
    return item;
  }

  dispose(): void {
    if (this.timer) {
      clearInterval(this.timer);
    }
    this.changed.dispose();
  }
}
