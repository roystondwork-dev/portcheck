// portcheck: Network Tools. A panel where you type a host and ports, or a
// domain or IP, and see the results. The logic is in netPanelModel.ts; this
// file only connects it to a webview and to portcheck.

import { randomBytes } from "node:crypto";
import * as vscode from "vscode";
import type { Deps } from "./commands";
import {
  addressesResult,
  checkResult,
  devicesResult,
  errorResult,
  isLocal,
  lookupResult,
  markChanges,
  parseHost,
  parseLookup,
  parsePorts,
  parseRequest,
  portSpec,
  projectMissingResult,
  projectResult,
  type Request,
  type Result,
  rowStates,
  scanResult,
  type Section,
  watchInterval,
} from "./netPanelModel";
import type { ProjectPorts } from "./project";

const SECTION_OF: Record<Request["type"], Section | undefined> = {
  check: "ports",
  lookup: "lookup",
  addresses: "addresses",
  devices: "devices",
  copy: undefined,
  stop: undefined,
  watch: "ports",
  unwatch: undefined,
  project: "project",
};

interface Watch {
  host: string;
  ports: number[];
  timer?: ReturnType<typeof setTimeout>;
  states?: Map<string, string>;
  last?: Result;
}

export class NetPanel implements vscode.Disposable {
  private panel: vscode.WebviewPanel | undefined;
  /** The last port check, to run again after a Stop. */
  private lastCheck: Request | undefined;
  /** Settles once the webview's script is listening. */
  private ready: Promise<void> = Promise.resolve();
  private readonly replied = new vscode.EventEmitter<Result>();
  /** Fires with each result the panel shows (tests wait on it). */
  readonly onDidReply = this.replied.event;
  private watch: Watch | undefined;

  constructor(
    private readonly d: Deps,
    private readonly extensionUri: vscode.Uri,
    private readonly project: ProjectPorts,
  ) {}

  /** True while a check is being watched. */
  get watching(): boolean {
    return this.watch !== undefined;
  }

  /** Opens the panel, or brings it to the front. */
  show(): void {
    if (this.panel) {
      this.panel.reveal();
      return;
    }
    const media = vscode.Uri.joinPath(this.extensionUri, "media");
    const panel = vscode.window.createWebviewPanel(
      "portcheck.networkTools",
      "portcheck: Network Tools",
      vscode.ViewColumn.Active,
      {
        enableScripts: true,
        retainContextWhenHidden: true,
        localResourceRoots: [media],
      },
    );
    panel.iconPath = vscode.Uri.joinPath(media, "ports.svg");
    let markReady = () => {};
    this.ready = new Promise((resolve) => {
      markReady = resolve;
    });
    panel.webview.html = html(panel.webview, media);
    panel.webview.onDidReceiveMessage(async (msg: unknown) => {
      if ((msg as { type?: unknown } | null)?.type === "ready") {
        markReady();
        return;
      }
      const reply = await this.handle(msg);
      if (reply) {
        this.post(reply);
      }
    });
    panel.onDidDispose(() => {
      this.stopWatch();
      this.panel = undefined;
    });
    this.panel = panel;
  }

  /**
   * Opens the panel and runs a request in it as if typed there, e.g.
   * { type: "check", host: "db.internal", ports: "5432" } or
   * { type: "lookup", text: "example.com" }. Returns false for anything
   * that isn't a request the panel can run.
   */
  async run(msg: unknown): Promise<boolean> {
    const req = parseRequest(msg);
    if (req === undefined || req.type === "copy") {
      return false;
    }
    this.show();
    await this.ready;
    return (
      (await this.panel?.webview.postMessage({
        type: "fill",
        request: req,
      })) === true
    );
  }

  /**
   * Answers one request from the webview. Exposed for tests, which call it
   * directly instead of clicking in the webview.
   */
  async handle(msg: unknown): Promise<Result | undefined> {
    const req = parseRequest(msg);
    if (req === undefined) {
      return undefined;
    }
    const section = SECTION_OF[req.type];
    try {
      return await this.answer(req);
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err);
      this.d.log.error(`network tools: ${message}`);
      return section === undefined ? undefined : errorResult(section, message);
    }
  }

  private async answer(req: Request): Promise<Result | undefined> {
    const client = this.d.client();
    switch (req.type) {
      case "check":
      case "watch": {
        const host = parseHost(req.host);
        if (typeof host !== "string") {
          return errorResult("ports", host.error);
        }
        const ports = parsePorts(req.ports);
        if (typeof ports === "string") {
          return errorResult("ports", ports);
        }
        // A new check or watch replaces the one being watched.
        this.stopWatch();
        this.lastCheck = { type: "check", host: req.host, ports: req.ports };
        if (req.type === "check") {
          return this.check(host, ports);
        }
        this.watch = { host, ports };
        return this.tick(this.watch);
      }
      case "unwatch": {
        const last = this.watch?.last;
        this.stopWatch();
        return last && { ...last, watching: false };
      }
      case "project":
        return this.projectResult();
      case "lookup": {
        const queries = parseLookup(req.text);
        return typeof queries === "string"
          ? errorResult("lookup", queries)
          : lookupResult(await client.lookup(queries));
      }
      case "addresses":
        return addressesResult(await client.net(req.public));
      case "devices": {
        if (req.probe) {
          const ok = await this.d
            .ui()
            .confirm(
              "Probe your local network for more devices?",
              "portcheck sends connection attempts to every address on your local network (only private networks, rate-limited). Only probe networks you own or are allowed to test.",
              "Probe",
            );
          if (ok !== "Probe") {
            return undefined;
          }
        }
        return devicesResult(await client.devices(req.probe));
      }
      case "copy":
        await vscode.env.clipboard.writeText(req.text);
        void this.d.ui().info(`Copied ${req.text}.`);
        return undefined;
      case "stop": {
        // The same confirmation and safety guards as the Stop command.
        await vscode.commands.executeCommand("portcheck.stopPort", req.port);
        if (this.project.config !== undefined) {
          this.post(await this.projectResult());
        }
        if (this.watch) {
          return undefined; // the next tick shows it
        }
        return this.lastCheck ? this.answer(this.lastCheck) : undefined;
      }
    }
  }

  /** Shows a result in the panel. */
  private post(result: Result): void {
    void this.panel?.webview.postMessage(result);
    this.replied.fire(result);
  }

  private async check(host: string, ports: number[]): Promise<Result> {
    const client = this.d.client();
    if (isLocal(host)) {
      // On this machine: who holds each port, not just whether it answers.
      const data = await Promise.all(ports.map((p) => client.check(p)));
      return checkResult({ ports: data.flatMap((x) => x.ports) });
    }
    return scanResult(await client.scan(host, portSpec(ports)));
  }

  /**
   * Runs a watched check, notes what changed, and schedules the next run,
   * whose result is posted to the panel.
   */
  private async tick(w: Watch): Promise<Result | undefined> {
    let result: Result;
    try {
      result = await this.check(w.host, w.ports);
    } catch (err) {
      result = errorResult(
        "ports",
        err instanceof Error ? err.message : String(err),
      );
    }
    if (this.watch !== w) {
      return undefined; // stopped while this run was in flight
    }
    const changes = markChanges(w.states, result, w.host);
    w.states = rowStates(result.rows, w.host);
    const every = watchInterval(w.host) / 1000;
    result.watching = true;
    result.summary = `Watching: checks every ${every} s. ${result.summary ?? ""}`;
    w.last = result;
    if (changes.length > 0) {
      void this.d.ui().info(`portcheck: ${changes.join("; ")}`);
    }
    w.timer = setTimeout(() => {
      void this.tick(w).then((r) => {
        if (r) {
          this.post(r);
        }
      });
    }, watchInterval(w.host));
    return result;
  }

  private stopWatch(): void {
    if (this.watch?.timer !== undefined) {
      clearTimeout(this.watch.timer);
    }
    this.watch = undefined;
  }

  private async projectResult(): Promise<Result> {
    await this.project.refresh();
    return this.project.data
      ? projectResult(this.project.data)
      : projectMissingResult(this.project.config !== undefined);
  }

  dispose(): void {
    this.stopWatch();
    this.panel?.dispose();
    this.replied.dispose();
  }
}

function html(webview: vscode.Webview, media: vscode.Uri): string {
  const nonce = randomBytes(16).toString("hex");
  const script = webview.asWebviewUri(
    vscode.Uri.joinPath(media, "netPanel.js"),
  );
  const style = webview.asWebviewUri(
    vscode.Uri.joinPath(media, "netPanel.css"),
  );
  return `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta http-equiv="Content-Security-Policy" content="default-src 'none'; style-src ${webview.cspSource}; script-src 'nonce-${nonce}';">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<link rel="stylesheet" href="${style.toString()}">
<title>Network Tools</title>
</head>
<body>
<main>
  <h1>Network Tools</h1>

  <section id="ports">
    <h2>Check ports</h2>
    <p class="hint">On this machine, see what holds each port. On another host, see which ports accept connections. Only check hosts you own or are allowed to test.</p>
    <form data-request="check">
      <label>Host <input name="host" value="localhost" spellcheck="false" autocomplete="off"></label>
      <label>Ports <input name="ports" placeholder="3000, 80,443 or 8000-8100" spellcheck="false" autocomplete="off" required></label>
      <button type="submit">Check</button>
      <button type="button" class="secondary" data-watch title="Check again every few seconds and say when something changes">Watch</button>
    </form>
    <div class="out" aria-live="polite"></div>
  </section>

  <section id="project">
    <h2>Project ports</h2>
    <p class="hint">The ports your workspace's .portcheck.yaml names, and whether each is as expected.</p>
    <div class="actions">
      <button class="secondary" data-request="project">Check again</button>
    </div>
    <div class="out" aria-live="polite"></div>
  </section>

  <section id="lookup">
    <h2>Look up a domain or IP</h2>
    <form data-request="lookup">
      <label class="wide">Domains or IPs <input name="text" placeholder="example.com 192.168.1.20" spellcheck="false" autocomplete="off" required></label>
      <button type="submit">Look up</button>
    </form>
    <div class="out" aria-live="polite"></div>
  </section>

  <section id="addresses">
    <h2>My addresses</h2>
    <div class="actions">
      <button data-request="addresses">Show</button>
      <button class="secondary" data-request="addresses" data-public="true" title="Asks ipify.org: the only part that contacts the internet">Show public address too</button>
    </div>
    <div class="out" aria-live="polite"></div>
  </section>

  <section id="devices">
    <h2>Devices on my network</h2>
    <div class="actions">
      <button data-request="devices">List devices</button>
      <button class="secondary" data-request="devices" data-probe="true" title="Sends connection attempts to your local network; asks first">Probe for more…</button>
    </div>
    <div class="out" aria-live="polite"></div>
  </section>
</main>
<script nonce="${nonce}" src="${script.toString()}"></script>
</body>
</html>`;
}
