import { chmodSync, mkdtempSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import * as path from "node:path";
import { describe, expect, it } from "vitest";
import { PortcheckError } from "./cli";
import { nodeExec } from "./nodeExec";

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

  it.skipIf(process.platform === "win32")("reports other start failures", async () => {
    const dir = mkdtempSync(path.join(tmpdir(), "pc-"));
    const f = path.join(dir, "not-executable");
    writeFileSync(f, "#!/bin/sh\n");
    chmodSync(f, 0o644);
    await expect(nodeExec(f, [], 10_000)).rejects.toThrow(/EACCES/);
  });
});
