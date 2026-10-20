import { readFileSync } from "node:fs";
import * as path from "node:path";
import { describe, expect, it } from "vitest";
import { type ExecFn, type ExecResult, PortcheckClient, PortcheckError, resolveBinary } from "./cli";


// Golden JSON produced by the Go CLI's own tests: if the CLI's JSON changes,
// these tests notice.
const golden = (name: string) =>
  readFileSync(path.join(__dirname, "..", "..", "..", "internal", "output", "testdata", `${name}.json.golden`), "utf8");

function fakeExec(result: Partial<ExecResult>, calls: string[][] = []): ExecFn {
  return (file, args, timeoutMs) => {
    calls.push([file, String(timeoutMs), ...args]);
    return Promise.resolve({ stdout: "", stderr: "", code: 0, ...result });
  };
}

describe("resolveBinary", () => {
  const base = { extensionPath: "/ext", platform: "darwin" as NodeJS.Platform, arch: "arm64", exists: () => false };
  it("prefers the configured path", () => {
    expect(resolveBinary({ ...base, configured: "  /opt/portcheck  " })).toBe("/opt/portcheck");
  });
  it("uses the binary bundled for this OS and CPU", () => {
    const seen: string[] = [];
    const exists = (f: string) => (seen.push(f), true);
    expect(resolveBinary({ ...base, configured: "", exists })).toBe(path.join("/ext", "bin", "darwin-arm64", "portcheck"));
    expect(resolveBinary({ ...base, configured: "", platform: "win32", arch: "x64", exists })).toBe(
      path.join("/ext", "bin", "win32-x64", "portcheck.exe"),
    );
    expect(seen).toHaveLength(2);
  });
  it("falls back to PATH", () => {
    expect(resolveBinary({ ...base, configured: " " })).toBe("portcheck");
    expect(resolveBinary({ ...base, configured: "", platform: "win32" })).toBe("portcheck.exe");
  });
});

describe("PortcheckClient", () => {
  it("parses real list output", async () => {
    const calls: string[][] = [];
    const c = new PortcheckClient("pc", fakeExec({ stdout: golden("list") }, calls), 1234);
    const data = await c.list();
    expect(calls[0]).toEqual(["pc", "1234", "list", "--listening", "--json"]);
    expect(data.partial).toBe(true);
    const web = data.sockets.find((s) => s.container !== undefined);
    expect(web?.container?.stopHint).toBe("docker stop web");
    expect(data.sockets.find((s) => s.process?.name === "node")?.process?.ancestors?.[0]).toEqual({ pid: 4200, name: "npm" });
  });

  it("builds the arguments of every command", async () => {
    const calls: string[][] = [];
    const c = new PortcheckClient("pc", (f, a, t) => {
      calls.push([...a]);
      const kind = a[0] ?? "";
      return fakeExec({ stdout: JSON.stringify({ schemaVersion: 1, kind, data: {} }) })(f, a, t);
    });
    await c.check(3000);
    await c.check(53, "udp");
    await c.free("3000-3999");
    await c.free("1-2", 3);
    await c.kill(42);
    await c.kill(42, true);
    await c.version();
    await c.doctor("/w/.portcheck.yaml");
    expect(calls).toEqual([
      ["check", "3000", "--proto", "tcp", "--json"],
      ["check", "53", "--proto", "udp", "--json"],
      ["free", "3000-3999", "-n", "1", "--json"],
      ["free", "1-2", "-n", "3", "--json"],
      ["kill", "--pid", "42", "--yes", "--json"],
      ["kill", "--pid", "42", "--yes", "--force", "--json"],
      ["version", "--json"],
      ["doctor", "--config", "/w/.portcheck.yaml", "--json"],
    ]);
  });

  it("accepts expected non-zero exit codes", async () => {
    const c = new PortcheckClient("pc", fakeExec({ stdout: golden("check"), code: 1 }));
    const data = await c.check(3000);
    expect(data.ports[0]?.status).toBe("in-use");
  });

  it("reports failures from stderr", async () => {
    const c = new PortcheckClient("pc", fakeExec({ stderr: "\nportcheck: unknown flag: --x\nRun 'portcheck --help'", code: 2 }));
    await expect(c.list()).rejects.toThrow(/^unknown flag: --x$/);
  });

  it("explains empty output with an expected exit code", async () => {
    const hint = 'Port 8080 is published by docker container "web" (nginx). Stop it there instead:';
    await expect(new PortcheckClient("pc", fakeExec({ stderr: hint, code: 1 })).kill(1)).rejects.toThrow(hint);
    await expect(new PortcheckClient("pc", fakeExec({ code: 1 })).kill(1)).rejects.toThrow("portcheck exited with status 1");
  });

  it("rejects invalid JSON, other schema versions and other kinds", async () => {
    const run = (stdout: string) => new PortcheckClient("pc", fakeExec({ stdout })).list();
    await expect(run("{oops")).rejects.toThrow('invalid JSON for "list"');
    await expect(run(JSON.stringify({ schemaVersion: 2, kind: "list", data: {} }))).rejects.toThrow("schema 2 is not supported");
    await expect(run(JSON.stringify({ schemaVersion: 1, kind: "free", data: {} }))).rejects.toThrow(
      'expected "list" output from portcheck, got "free"',
    );
  });

  it("keeps the exit code on errors", async () => {
    const err = await new PortcheckClient("pc", fakeExec({ code: 4, stderr: "permission denied" }))
      .list()
      .catch((e: unknown) => e);
    expect(err).toBeInstanceOf(PortcheckError);
    expect((err as PortcheckError).exitCode).toBe(4);
    expect((err as PortcheckError).name).toBe("PortcheckError");
    expect((err as PortcheckError).detail).toBe("permission denied");
  });

  it("keeps multi-line error detail without the usage hint", async () => {
    const stderr =
      "portcheck: /w/.portcheck.yaml: invalid .portcheck.yaml: yaml: unmarshal errors:\n  line 3: field prot not found\nRun 'portcheck doctor --help' for usage.\n";
    const err = (await new PortcheckClient("pc", fakeExec({ code: 2, stderr })).doctor("/w/.portcheck.yaml").catch((e: unknown) => e)) as PortcheckError;
    expect(err.message).toBe("/w/.portcheck.yaml: invalid .portcheck.yaml: yaml: unmarshal errors:");
    expect(err.detail).toBe("/w/.portcheck.yaml: invalid .portcheck.yaml: yaml: unmarshal errors:\n  line 3: field prot not found");
  });

  it("parses real doctor output", async () => {
    const data = await new PortcheckClient("pc", fakeExec({ stdout: golden("doctor"), code: 1 })).doctor("/c");
    expect(data.checks[0]).toMatchObject({ name: "web", ok: false, fix: "portcheck kill 3000" });
  });
});
