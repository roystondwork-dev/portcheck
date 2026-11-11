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
  parseHost,
  parseLookup,
  parsePorts,
  parseRequest,
  portSpec,
  type Request,
  type Result,
  scanResult,
  type Section,
} from "./netPanelModel";

const SECTION_OF: Record<Request["type"], Section | undefined> = {
  check: "ports",
  lookup: "lookup",
  addresses: "addresses",
  devices: "devices",
  copy: undefined,
  stop: undefined,
};

export class NetPanel implements vscode.Disposable {
  private panel: vscode.WebviewPanel | undefined;
  /** The last request per section, to run again after a Stop. */
  private lastCheck: Request | undefined;

  constructor(
    private readonly d: Deps,
    private readonly extensionUri: vscode.Uri,
  ) {}

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
    panel.webview.html = html(panel.webview, media);
    panel.webview.onDidReceiveMessage(async (msg: unknown) => {
      const reply = await this.handle(msg);
      if (reply) {
        void panel.webview.postMessage(reply);
      }
    });
    panel.onDidDispose(() => {
      this.panel = undefined;
    });
    this.panel = panel;
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
      case "check": {
        const host = parseHost(req.host);
        if (typeof host !== "string") {
          return errorResult("ports", host.error);
        }
        const ports = parsePorts(req.ports);
        if (typeof ports === "string") {
          return errorResult("ports", ports);
        }
        this.lastCheck = req;
        if (isLocal(host)) {
          // On this machine: who holds each port, not just whether it answers.
          const data = await Promise.all(ports.map((p) => client.check(p)));
          return checkResult({ ports: data.flatMap((x) => x.ports) });
        }
        return scanResult(await client.scan(host, portSpec(ports)));
      }
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
        return this.lastCheck ? this.answer(this.lastCheck) : undefined;
      }
    }
  }

  dispose(): void {
    this.panel?.dispose();
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
    </form>
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
