import { chmodSync, mkdtempSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import * as path from "node:path";
import { describe, expect, it } from "vitest";
import { PortcheckError } from "./cli";
import { nodeExec, settle } from "./nodeExec";

const node = process.execPath;

describe("nodeExec", () => {
  it("returns output of a successful run", async () => {
    await expect(nodeExec(node, ["-e", "console.log('hi')"], 10_000)).resolves.toEqual({ stdout: "hi\n", stderr: "", code: 0 });
  });

  it("returns non-zero exit codes instead of failing", async () => {
    const r = await nodeExec(node, ["-e", "console.log('out'); console.error('err'); process.exit(3)"], 10_000);
    expect(r).toEqual({ stdout: "out\n", stderr: "err\n", code: 3 });
  });

  it("explains a missing binary", async () => {
    await expect(nodeExec("portcheck-definitely-missing", [], 10_000)).rejects.toThrow(/was not found/);
  });

  it("reports timeouts", async () => {
    const err = await nodeExec(node, ["-e", "setTimeout(() => {}, 10000)"], 200).catch((e: unknown) => e);
    expect(err).toBeInstanceOf(PortcheckError);
    expect((err as Error).message).toBe("portcheck timed out after 200 ms");
  });

  it("maps every execFile outcome (all platforms)", () => {
    expect(settle(null, "o", "e", "pc", 1)).toEqual({ stdout: "o", stderr: "e", code: 0 });
    expect(settle(Object.assign(new Error("exit"), { code: 4 }), "o", "e", "pc", 1)).toEqual({ stdout: "o", stderr: "e", code: 4 });
    expect((settle(Object.assign(new Error("x"), { code: "ENOENT" }), "", "", "pc", 1) as Error).message).toMatch(/not found \("pc"\)/);
    expect((settle(Object.assign(new Error("x"), { killed: true }), "", "", "pc", 50) as Error).message).toBe("portcheck timed out after 50 ms");
    expect((settle(Object.assign(new Error("spawn EACCES"), { code: "EACCES" }), "", "", "pc", 1) as Error).message).toBe("spawn EACCES");
  });

  it.skipIf(process.platform === "win32")("reports other start failures from the OS", async () => {
    const dir = mkdtempSync(path.join(tmpdir(), "pc-"));
    const f = path.join(dir, "not-executable");
    writeFileSync(f, "#!/bin/sh\n");
    chmodSync(f, 0o644);
    await expect(nodeExec(f, [], 10_000)).rejects.toThrow(/EACCES/);
  });
});
