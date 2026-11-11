import * as assert from "node:assert/strict";
import * as vscode from "vscode";
import type { PortcheckApi } from "../../extension";
import type { PortRow } from "../../model";
import type { Result } from "../../netPanelModel";
import {
  activateExtension,
  close,
  listen,
  portOf,
  rowFor,
  ScriptedUI,
  spawnHolder,
} from "./helpers";

describe("portcheck extension", () => {
  let api: PortcheckApi;

  before(async () => {
    api = await activateExtension();
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
    assert.equal(
      await vscode.commands.executeCommand("portcheck.checkPort", port),
      "in-use",
    );
    assert.ok(ui.has(`Port ${port} is in use by`), ui.messages.join("\n"));
    await close(server);
    assert.equal(
      await vscode.commands.executeCommand("portcheck.checkPort", port),
      "free",
    );
    assert.ok(ui.has(`Port ${port} is free.`));
  });

  it("asks for a port when none is given, and stops if cancelled", async () => {
    const ui = new ScriptedUI([undefined]);
    api.setUI(ui);
    assert.equal(
      await vscode.commands.executeCommand("portcheck.checkPort"),
      undefined,
    );
    assert.ok(ui.has("input: Port to check"));
  });

  it("finds a free port and copies it", async () => {
    const ui = new ScriptedUI(["Copy"]);
    api.setUI(ui);
    const port = await vscode.commands.executeCommand<number>(
      "portcheck.findFreePort",
      "40000-40100",
    );
    assert.ok(port >= 40000 && port <= 40100, `port ${port}`);
    assert.equal(await vscode.env.clipboard.readText(), String(port));
  });

  it("stops a real process after confirmation", async () => {
    const { child, port } = await spawnHolder();
    const exited = new Promise<void>((resolve) =>
      child.once("exit", () => {
        resolve();
      }),
    );
    const row = await rowFor(api, port, child.pid);
    const ui = new ScriptedUI(["Stop"]);
    api.setUI(ui);
    const outcome = await vscode.commands.executeCommand("portcheck.stop", row);
    assert.equal(outcome, "terminated");
    assert.ok(
      ui.has(`confirm: Stop`) && ui.has(`(pid ${child.pid}) on port ${port}?`),
      ui.messages.join("\n"),
    );
    await exited;
  });

  it("does nothing when the confirmation is cancelled", async () => {
    const { child, port } = await spawnHolder();
    try {
      const row = await rowFor(api, port, child.pid);
      api.setUI(new ScriptedUI([undefined]));
      assert.equal(
        await vscode.commands.executeCommand("portcheck.stop", row),
        undefined,
      );
      assert.equal(
        child.exitCode,
        null,
        "process was stopped despite cancelling",
      );
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
      assert.equal(
        await vscode.commands.executeCommand("portcheck.stop", row),
        "failed",
      );
      assert.ok(ui.has("refusing to kill"), ui.messages.join("\n"));
    } finally {
      await close(server);
    }
  });

  it("frees a port by number via the quick pick", async () => {
    const { child, port } = await spawnHolder();
    const exited = new Promise<void>((resolve) =>
      child.once("exit", () => {
        resolve();
      }),
    );
    api.setUI(new ScriptedUI(["Stop"]));
    assert.equal(
      await vscode.commands.executeCommand("portcheck.stopPort", port),
      "terminated",
    );
    await exited;
  });

  it("reports when nothing listens on a port", async () => {
    const server = await listen();
    const port = portOf(server);
    await close(server);
    const ui = new ScriptedUI();
    api.setUI(ui);
    assert.equal(
      await vscode.commands.executeCommand("portcheck.stopPort", port),
      undefined,
    );
    assert.ok(ui.has(`Nothing is listening on port ${port}.`));
  });

  it("never kills container proxies; offers the stop command instead", async () => {
    const row: PortRow = {
      id: "x",
      port: 8080,
      proto: "tcp",
      kind: "container",
      label: ":8080  docker:web",
      description: "",
      tooltip: "",
      contextValue: "port.container.tcp",
      pid: 1,
      container: {
        name: "web",
        runtime: "docker",
        stopHint: "docker stop web",
      },
    };
    const ui = new ScriptedUI(["Copy Command"]);
    api.setUI(ui);
    assert.equal(
      await vscode.commands.executeCommand("portcheck.stop", row),
      "Copy Command",
    );
    assert.equal(await vscode.env.clipboard.readText(), "docker stop web");
  });

  it("explains hidden owners with an elevated command", async () => {
    const row: PortRow = {
      id: "y",
      port: 22,
      proto: "tcp",
      kind: "hidden",
      label: ":22  (owner hidden)",
      description: "",
      tooltip: "",
      contextValue: "port.hidden.tcp",
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
    await config.update(
      "path",
      "/nonexistent/portcheck",
      vscode.ConfigurationTarget.Workspace,
    );
    try {
      await api.view.refresh();
      assert.deepEqual(api.view.rows, []);
      assert.equal(
        await vscode.commands.executeCommand("portcheck.checkPort", 1),
        undefined,
      );
      assert.ok(
        ui.has("error: portcheck: portcheck was not found"),
        ui.messages.join("\n"),
      );
    } finally {
      await config.update("path", good, vscode.ConfigurationTarget.Workspace);
    }
    assert.equal(api.binary(), good);
  });

  it("accepts refresh interval changes", async () => {
    const config = vscode.workspace.getConfiguration("portcheck");
    await config.update(
      "refreshInterval",
      1,
      vscode.ConfigurationTarget.Workspace,
    );
    await config.update(
      "refreshInterval",
      0,
      vscode.ConfigurationTarget.Workspace,
    );
  });
  it("mentions a long-running dev server once, and can be turned off", async () => {
    // Pretend it's two days later, so the holder (running in this project) looks forgotten.
    const later = Date.now() + 48 * 3_600_000;
    const { child } = await spawnHolder();
    const config = vscode.workspace.getConfiguration("portcheck");
    try {
      // Dismissed: nothing is stopped. Answering "Stop" here could stop other
      // processes on a developer's machine that also look forgotten.
      const ui = new ScriptedUI([undefined]);
      api.setUI(ui);
      const found = await api.forgotten.check(later);
      assert.ok(
        found.includes(child.pid as number),
        `found ${found.join(",")}`,
      );
      assert.ok(ui.has("dev server"), ui.messages.join("\n"));
      // Mentioned once per window.
      assert.ok(
        !(await api.forgotten.check(later)).includes(child.pid as number),
      );

      const { child: second } = await spawnHolder();
      try {
        api.setUI(new ScriptedUI(["Don't Show Again"]));
        assert.ok(
          (await api.forgotten.check(later)).includes(second.pid as number),
        );
        // Settings objects are snapshots: read the setting again.
        assert.equal(
          vscode.workspace
            .getConfiguration("portcheck")
            .get<boolean>("forgottenServers.notify"),
          false,
        );
        assert.deepEqual(await api.forgotten.check(later), []);
      } finally {
        second.kill();
      }
    } finally {
      child.kill();
      await config.update(
        "forgottenServers.notify",
        undefined,
        vscode.ConfigurationTarget.Global,
      );
    }
  });

  it("shows this machine's addresses and copies one", async () => {
    const ui = new ScriptedUI(); // picks the first item: the primary address
    api.setUI(ui);
    const copied = await vscode.commands.executeCommand<string | undefined>(
      "portcheck.showIp",
    );
    assert.ok(ui.has("Show My Public Address"), ui.messages.join("\n"));
    if (copied !== undefined) {
      // A runner always has an address; it's copied as is.
      assert.equal(await vscode.env.clipboard.readText(), copied);
      assert.ok(ui.has(`Copied ${copied}.`), ui.messages.join("\n"));
    }
  });

  it("looks up an address and lists devices", async () => {
    const ui = new ScriptedUI([], ["127.0.0.1", undefined]);
    api.setUI(ui);
    assert.equal(
      await vscode.commands.executeCommand("portcheck.lookup", "localhost"),
      "127.0.0.1",
    );
    assert.equal(await vscode.env.clipboard.readText(), "127.0.0.1");
    // Cancelled: nothing is copied, and the probe is offered but not run.
    assert.equal(
      await vscode.commands.executeCommand("portcheck.networkDevices"),
      undefined,
    );
    assert.ok(ui.has("Probe for More Devices"), ui.messages.join("\n"));
  });

  describe("Network Tools panel", () => {
    /** The next result shown in a section (Project ports also load on open). */
    const nextReply = (section: string) =>
      new Promise<Result>((resolve) => {
        const sub = api.netTools.onDidReply((r) => {
          if (r.section === section) {
            sub.dispose();
            resolve(r);
          }
        });
      });

    before(async () => {
      await vscode.commands.executeCommand("portcheck.networkTools");
    });

    it("shows who holds a local port, and that a free one is free", async () => {
      const server = await listen();
      const port = portOf(server);
      try {
        const busy = await api.netTools.handle({
          type: "check",
          host: "localhost",
          ports: String(port),
        });
        assert.equal(busy?.section, "ports");
        const row = busy.rows.find((r) => r.copy === String(process.pid));
        assert.ok(row, JSON.stringify(busy));
        assert.equal(row.cells[1], "in use");
        assert.equal(row.stopPort, port);
      } finally {
        await close(server);
      }
      const free = await api.netTools.handle({
        type: "check",
        host: "",
        ports: String(port),
      });
      assert.deepEqual(free?.rows[0]?.cells.slice(0, 2), [
        `${port}/tcp`,
        "free",
      ]);
    });

    it("checks another host's ports by connecting", async () => {
      const server = await listen();
      const port = portOf(server);
      try {
        // The IPv4-mapped form of loopback isn't "this machine" to the panel,
        // so it takes the remote path (portcheck scan) but still reaches the
        // test server.
        const r = await api.netTools.handle({
          type: "check",
          host: "::ffff:127.0.0.1",
          ports: String(port),
        });
        assert.deepEqual(r?.columns, ["Port", "Status", "Details", "Host"]);
        assert.deepEqual(r.rows[0]?.cells.slice(0, 2), [String(port), "open"]);
        assert.equal(r.rows[0].tone, "ok");
      } finally {
        await close(server);
      }
    });

    it("explains bad input instead of running anything", async () => {
      const r = await api.netTools.handle({
        type: "check",
        host: "10.0.0.0/24",
        ports: "80",
      });
      assert.match(r?.error ?? "", /one host name/);
      const p = await api.netTools.handle({
        type: "check",
        host: "localhost",
        ports: "http",
      });
      assert.match(p?.error ?? "", /not a port/);
      assert.equal(await api.netTools.handle({ type: "nope" }), undefined);
    });

    it("looks up a name and lists this machine's addresses", async () => {
      const r = await api.netTools.handle({
        type: "lookup",
        text: "localhost",
      });
      assert.equal(r?.section, "lookup");
      assert.ok(
        r.rows.some((x) => x.copy === "127.0.0.1" || x.copy === "::1"),
        JSON.stringify(r),
      );
      const a = await api.netTools.handle({ type: "addresses" });
      assert.equal(a?.section, "addresses");
      assert.equal(a.error, undefined);
    });

    it("asks before probing the network, and does nothing if cancelled", async () => {
      const ui = new ScriptedUI([undefined]);
      api.setUI(ui);
      assert.equal(
        await api.netTools.handle({ type: "devices", probe: true }),
        undefined,
      );
      assert.ok(
        ui.has("confirm: Probe your local network"),
        ui.messages.join("\n"),
      );
    });

    it("runs a request given to the command in the real webview", async () => {
      const server = await listen();
      const port = portOf(server);
      try {
        const reply = nextReply("ports");
        assert.equal(
          await vscode.commands.executeCommand("portcheck.networkTools", {
            type: "check",
            host: "localhost",
            ports: String(port),
          }),
          true,
        );
        // The webview typed the values, submitted the form, and got this back.
        const r = await reply;
        assert.equal(r.section, "ports");
        assert.ok(
          r.rows.some((x) => x.stopPort === port),
          JSON.stringify(r),
        );
      } finally {
        await close(server);
      }
      assert.equal(
        await vscode.commands.executeCommand("portcheck.networkTools", {
          type: "copy",
          text: "x",
        }),
        false,
      );
    });

    it("watches a port and says when it changes", async () => {
      const server = await listen();
      const port = portOf(server);
      const ui = new ScriptedUI();
      api.setUI(ui);
      const first = await api.netTools.handle({
        type: "watch",
        host: "localhost",
        ports: String(port),
      });
      try {
        assert.equal(first?.watching, true);
        assert.match(first.summary ?? "", /^Watching: checks every 2 s\. /);
        assert.equal(first.rows[0]?.cells[1], "in use");
        assert.ok(api.netTools.watching);
        // Free the port; the next check (2 s later) notices.
        const next = nextReply("ports");
        await close(server);
        const r = await next;
        assert.equal(r.rows[0]?.cells[1], "free");
        assert.equal(r.rows[0].changed, true);
        assert.ok(ui.has(`${port}/tcp is now free`), ui.messages.join("\n"));
      } finally {
        const stopped = await api.netTools.handle({ type: "unwatch" });
        assert.equal(stopped?.watching, false);
        assert.equal(api.netTools.watching, false);
      }
    });

    it("copies a value", async () => {
      api.setUI(new ScriptedUI());
      await api.netTools.handle({ type: "copy", text: "192.0.2.1" });
      assert.equal(await vscode.env.clipboard.readText(), "192.0.2.1");
    });
  });
});
