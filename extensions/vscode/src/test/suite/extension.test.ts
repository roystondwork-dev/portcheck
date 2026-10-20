import * as assert from "node:assert/strict";
import { type ChildProcess, spawn } from "node:child_process";
import * as net from "node:net";
import * as vscode from "vscode";
import type { UI } from "../../commands";
import type { PortcheckApi } from "../../extension";
import type { PortRow } from "../../model";

/** Records messages and returns scripted answers in order. */
class ScriptedUI implements UI {
  readonly messages: string[] = [];
  constructor(private readonly answers: (string | undefined)[] = []) {}
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
    this.messages.push(`pick: ${items.map((i) => i.label).join(", ")}`);
    return Promise.resolve(items[0]);
  }
  has(fragment: string): boolean {
    return this.messages.some((m) => m.includes(fragment));
  }
}

function listen(): Promise<net.Server> {
  return new Promise((resolve) => {
    const s = net.createServer().listen(0, "127.0.0.1", () => {
      resolve(s);
    });
  });
}

function portOf(s: net.Server): number {
  return (s.address() as net.AddressInfo).port;
}

function close(s: net.Server): Promise<void> {
  return new Promise((resolve) => s.close(() => {
    resolve();
  }));
}

/** Starts a separate node process holding a TCP port; resolves with it and the port. */
function spawnHolder(): Promise<{ child: ChildProcess; port: number }> {
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

async function rowFor(api: PortcheckApi, port: number, pid?: number): Promise<PortRow> {
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

describe("portcheck extension", () => {
  let api: PortcheckApi;

  before(async () => {
    const ext = vscode.extensions.getExtension<PortcheckApi>("roystondwork-dev.portcheck");
    assert.ok(ext, "extension not found");
    api = await ext.activate();
  });

  it("uses the configured binary and reports its version", async () => {
    assert.equal(api.binary(), process.env.PORTCHECK_TEST_BINARY);
    const v = await api.client().version();
    assert.match(v.platform, /\//);
  });

  it("lists a real listener with its process", async () => {
    const server = await listen();
    try {
      const row = await rowFor(api, portOf(server), process.pid);
      assert.equal(row.kind, "process");
      assert.equal(row.proto, "tcp");
      assert.equal(row.contextValue, "port.process.tcp");
      assert.ok(row.tooltip.includes(`pid ${process.pid}`));
    } finally {
      await close(server);
    }
  });

  it("checks ports: in use, then free", async () => {
    const server = await listen();
    const port = portOf(server);
    const ui = new ScriptedUI();
    api.setUI(ui);
    assert.equal(await vscode.commands.executeCommand("portcheck.checkPort", port), "in-use");
    assert.ok(ui.has(`Port ${port} is in use by`), ui.messages.join("\n"));
    await close(server);
    assert.equal(await vscode.commands.executeCommand("portcheck.checkPort", port), "free");
    assert.ok(ui.has(`Port ${port} is free.`));
  });

  it("asks for a port when none is given, and stops if cancelled", async () => {
    const ui = new ScriptedUI([undefined]);
    api.setUI(ui);
    assert.equal(await vscode.commands.executeCommand("portcheck.checkPort"), undefined);
    assert.ok(ui.has("input: Port to check"));
  });

  it("finds a free port and copies it", async () => {
    const ui = new ScriptedUI(["Copy"]);
    api.setUI(ui);
    const port = await vscode.commands.executeCommand<number>("portcheck.findFreePort", "40000-40100");
    assert.ok(port >= 40000 && port <= 40100, `port ${port}`);
    assert.equal(await vscode.env.clipboard.readText(), String(port));
  });

  it("stops a real process after confirmation", async () => {
    const { child, port } = await spawnHolder();
    const exited = new Promise<void>((resolve) => child.once("exit", () => {
      resolve();
    }));
    const row = await rowFor(api, port, child.pid);
    const ui = new ScriptedUI(["Stop"]);
    api.setUI(ui);
    const outcome = await vscode.commands.executeCommand("portcheck.stop", row);
    assert.equal(outcome, "terminated");
    assert.ok(ui.has(`confirm: Stop`) && ui.has(`(pid ${child.pid}) on port ${port}?`), ui.messages.join("\n"));
    await exited;
  });

  it("does nothing when the confirmation is cancelled", async () => {
    const { child, port } = await spawnHolder();
    try {
      const row = await rowFor(api, port, child.pid);
      api.setUI(new ScriptedUI([undefined]));
      assert.equal(await vscode.commands.executeCommand("portcheck.stop", row), undefined);
      assert.equal(child.exitCode, null, "process was stopped despite cancelling");
    } finally {
      child.kill();
    }
  });

  it("refuses to stop VS Code itself (portcheck's safety guards apply)", async () => {
    const server = await listen();
    try {
      const row = await rowFor(api, portOf(server), process.pid);
      const ui = new ScriptedUI(["Stop"]);
      api.setUI(ui);
      assert.equal(await vscode.commands.executeCommand("portcheck.stop", row), "failed");
      assert.ok(ui.has("refusing to kill"), ui.messages.join("\n"));
    } finally {
      await close(server);
    }
  });

  it("frees a port by number via the quick pick", async () => {
    const { child, port } = await spawnHolder();
    const exited = new Promise<void>((resolve) => child.once("exit", () => {
      resolve();
    }));
    api.setUI(new ScriptedUI(["Stop"]));
    assert.equal(await vscode.commands.executeCommand("portcheck.stopPort", port), "terminated");
    await exited;
  });

  it("reports when nothing listens on a port", async () => {
    const server = await listen();
    const port = portOf(server);
    await close(server);
    const ui = new ScriptedUI();
    api.setUI(ui);
    assert.equal(await vscode.commands.executeCommand("portcheck.stopPort", port), undefined);
    assert.ok(ui.has(`Nothing is listening on port ${port}.`));
  });

  it("never kills container proxies; offers the stop command instead", async () => {
    const row: PortRow = {
      id: "x", port: 8080, proto: "tcp", kind: "container", label: ":8080  docker:web", description: "",
      tooltip: "", contextValue: "port.container.tcp", pid: 1,
      container: { name: "web", runtime: "docker", stopHint: "docker stop web" },
    };
    const ui = new ScriptedUI(["Copy Command"]);
    api.setUI(ui);
    assert.equal(await vscode.commands.executeCommand("portcheck.stop", row), "Copy Command");
    assert.equal(await vscode.env.clipboard.readText(), "docker stop web");
  });

  it("explains hidden owners with an elevated command", async () => {
    const row: PortRow = {
      id: "y", port: 22, proto: "tcp", kind: "hidden", label: ":22  (owner hidden)", description: "",
      tooltip: "", contextValue: "port.hidden.tcp",
    };
    api.setUI(new ScriptedUI(["Copy Command"]));
    await vscode.commands.executeCommand("portcheck.stop", row);
    assert.match(await vscode.env.clipboard.readText(), /kill 22$/);
  });

  it("copies a PID", async () => {
    api.setUI(new ScriptedUI());
    const row = { pid: 4242 } as PortRow;
    await vscode.commands.executeCommand("portcheck.copyPid", row);
    assert.equal(await vscode.env.clipboard.readText(), "4242");
  });

  it("shows errors instead of failing when portcheck can't run", async () => {
    const config = vscode.workspace.getConfiguration("portcheck");
    const good = config.get<string>("path");
    const ui = new ScriptedUI();
    api.setUI(ui);
    await config.update("path", "/nonexistent/portcheck", vscode.ConfigurationTarget.Workspace);
    try {
      await api.view.refresh();
      assert.deepEqual(api.view.rows, []);
      assert.equal(await vscode.commands.executeCommand("portcheck.checkPort", 1), undefined);
      assert.ok(ui.has("error: portcheck: portcheck was not found"), ui.messages.join("\n"));
    } finally {
      await config.update("path", good, vscode.ConfigurationTarget.Workspace);
    }
    assert.equal(api.binary(), good);
  });

  it("accepts refresh interval changes", async () => {
    const config = vscode.workspace.getConfiguration("portcheck");
    await config.update("refreshInterval", 1, vscode.ConfigurationTarget.Workspace);
    await config.update("refreshInterval", 0, vscode.ConfigurationTarget.Workspace);
  });
});
