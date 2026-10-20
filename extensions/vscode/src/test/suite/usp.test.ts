import * as assert from "node:assert/strict";
import * as vscode from "vscode";
import type { PortcheckApi } from "../../extension";
import { activateExtension, close, listen, portOf, ScriptedUI, spawnHolder, waitFor } from "./helpers";

describe("terminal: port in use → fix", () => {
  let api: PortcheckApi;
  before(async () => {
    api = await activateExtension();
  });

  it("turns error lines into links, and nothing else", () => {
    const terminal = {} as vscode.Terminal;
    const links = api.terminal.provideTerminalLinks({ line: "Error: listen EADDRINUSE: address already in use :::3000", terminal });
    assert.equal(links.length, 1);
    assert.deepEqual([links[0]?.port, links[0]?.startIndex], [3000, "Error: listen ".length]);
    assert.equal(links[0]?.tooltip, "Fix port 3000 (portcheck)");
    assert.deepEqual(api.terminal.provideTerminalLinks({ line: "ready on http://localhost:3000", terminal }), []);
  });

  it("stops the holder from the fix menu", async () => {
    const { child, port } = await spawnHolder();
    const exited = new Promise<void>((resolve) => child.once("exit", () => {
      resolve();
    }));
    const ui = new ScriptedUI([], [`(pid ${child.pid})`]);
    api.setUI(ui);
    assert.equal(await api.terminal.fix(port, undefined), "stop");
    assert.ok(new RegExp(`^pick: \\$\\(debug-stop\\) Stop .* \\(pid ${child.pid}\\) \\[started by `).test(ui.messages[0] ?? ""), ui.messages.join("\n"));
    assert.ok(ui.has("$(copy) Copy free port"), ui.messages.join("\n"));
    await exited;
  });

  it("copies a free port instead", async () => {
    const server = await listen();
    const port = portOf(server);
    try {
      api.setUI(new ScriptedUI([], ["Copy free port"]));
      assert.equal(await api.terminal.fix(port, undefined), "copyPort");
      const copied = Number(await vscode.env.clipboard.readText());
      assert.ok(copied > port && copied <= port + 100, `copied ${copied}`);
    } finally {
      await close(server);
    }
  });

  it("does nothing when the menu is dismissed", async () => {
    const server = await listen();
    try {
      api.setUI(new ScriptedUI([], [undefined]));
      assert.equal(await api.terminal.fix(portOf(server), undefined), undefined);
    } finally {
      await close(server);
    }
  });

  it("notices the error in a real terminal and offers to stop and re-run", async function () {
    const server = await listen();
    const port = portOf(server);
    const terminal = vscode.window.createTerminal({ name: "portcheck-test" });
    try {
      const integrated = await new Promise<boolean>((resolve) => {
        if (terminal.shellIntegration) {
          resolve(true);
          return;
        }
        const sub = vscode.window.onDidChangeTerminalShellIntegration((e) => {
          if (e.terminal === terminal) {
            sub.dispose();
            resolve(true);
          }
        });
        setTimeout(() => {
          sub.dispose();
          resolve(false);
        }, 15_000);
      });
      if (!integrated || !terminal.shellIntegration) {
        this.skip(); // shell integration isn't available for this shell/CI image
      }
      const ui = new ScriptedUI([undefined]);
      api.setUI(ui);
      const command = `node -e "require('net').createServer().listen(${port}, '127.0.0.1')"`;
      const done = new Promise<void>((resolve) => {
        const sub = vscode.window.onDidEndTerminalShellExecution((e) => {
          if (e.terminal === terminal) {
            sub.dispose();
            resolve();
          }
        });
      });
      terminal.shellIntegration.executeCommand(command);
      await waitFor(() => ui.has(`warn: Port ${port} is already in use by`), "the notification");
      assert.ok(ui.has(`(pid ${process.pid})`), ui.messages.join("\n"));
      await done;

      // The failed command is known now, so the menu offers to re-run it on a free port.
      const menu = new ScriptedUI([], [undefined]);
      api.setUI(menu);
      await api.terminal.fix(port, terminal);
      assert.ok(menu.has("$(debug-restart) Stop "), menu.messages.join("\n"));
      assert.ok(menu.has("$(arrow-right) Re-run on free port"), menu.messages.join("\n"));
      assert.ok(menu.has("require('net')"), menu.messages.join("\n"));
    } finally {
      terminal.dispose();
      await close(server);
    }
  });
});

describe("project ports (.portcheck.yaml)", () => {
  let api: PortcheckApi;
  let file: vscode.Uri;
  before(async () => {
    api = await activateExtension();
    const [folder] = vscode.workspace.workspaceFolders ?? [];
    assert.ok(folder, "no workspace folder");
    file = vscode.Uri.joinPath(folder.uri, ".portcheck.yaml");
  });
  after(async () => {
    await vscode.workspace.fs.delete(file).then(undefined, () => undefined);
    await api.project.refresh();
  });

  const write = (text: string) => vscode.workspace.fs.writeFile(file, new TextEncoder().encode(text));

  it("reports a busy project port in the status bar, Problems and the view", async () => {
    const server = await listen();
    const port = portOf(server);
    try {
      await write(`version: 1\nports:\n  - name: api\n    port: ${port}\n`);
      await api.project.refresh();
      assert.equal(api.project.data?.ok, false);
      const diags = vscode.languages.getDiagnostics(file);
      assert.equal(diags.length, 1);
      const [d0] = diags;
      assert.ok(d0);
      assert.equal(d0.range.start.line, 2);
      assert.equal(d0.severity, vscode.DiagnosticSeverity.Warning);
      assert.match(d0.message, new RegExp(`^api \\(port ${port}\\): in use by `));

      await api.view.refresh();
      const row = api.view.rows[0];
      assert.equal(row?.port, port, "project ports come first");
      assert.equal(row.project, "api");

      // The status bar's fix: stopping our own process is refused by portcheck's guards.
      const ui = new ScriptedUI(["Stop"], ["api"]);
      api.setUI(ui);
      assert.equal(await vscode.commands.executeCommand("portcheck.doctor"), "api");
      assert.ok(ui.has("refusing to kill"), ui.messages.join("\n"));
    } finally {
      await close(server);
    }
    await api.project.refresh();
    assert.deepEqual(api.project.data.checks.map((c) => c.ok), [true]);
    assert.equal(vscode.languages.getDiagnostics(file).length, 0);
    api.setUI(new ScriptedUI());
    assert.equal(await vscode.commands.executeCommand("portcheck.doctor"), "ok");
  });

  it("shows errors in an invalid file at their line", async () => {
    await write("version: 1\nports:\n  - name: api\n    prot: 3000\n");
    await api.project.refresh();
    assert.equal(api.project.data, undefined);
    const diags = vscode.languages.getDiagnostics(file);
    assert.ok(diags.some((d) => d.range.start.line === 3 && d.severity === vscode.DiagnosticSeverity.Error), JSON.stringify(diags));
  });

  it("goes quiet without a file", async () => {
    await vscode.workspace.fs.delete(file);
    await api.project.refresh();
    assert.equal(api.project.config, undefined);
    assert.equal(vscode.languages.getDiagnostics(file).length, 0);
  });
});

describe("view filters", () => {
  it("hides UDP by default and toggles it", async () => {
    const api = await activateExtension();
    await api.view.refresh();
    assert.ok(!api.view.rows.some((r) => r.proto === "udp"));
    await vscode.commands.executeCommand("portcheck.toggleUdp");
    assert.equal(vscode.workspace.getConfiguration("portcheck").get("showUdp"), true);
    await vscode.commands.executeCommand("portcheck.toggleUdp");
    assert.equal(vscode.workspace.getConfiguration("portcheck").get("showUdp"), false);
  });
});
