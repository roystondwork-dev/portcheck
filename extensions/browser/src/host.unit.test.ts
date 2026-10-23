import { describe, expect, it } from "vitest";
import { checkGolden } from "./fixtures";
import { call, HOST, HostError, type SendNative } from "./host";

const answer = (resp: unknown): SendNative => () => Promise.resolve(resp);
const failing = (message: string): SendNative => () => Promise.reject(new Error(message));

describe("call", () => {
  it("returns the data of real check output", async () => {
    const sent: unknown[] = [];
    const send: SendNative = (host, msg) => {
      sent.push([host, msg]);
      return Promise.resolve({ ok: true, exitCode: 1, data: checkGolden });
    };
    const a = await call(send, { op: "check", port: 3000 }, "check");
    expect(sent).toEqual([[HOST, { op: "check", port: 3000 }]]);
    expect(a.exitCode).toBe(1);
    expect(a.data).toEqual(checkGolden.data);
  });

  it("defaults a missing exit code to 0", async () => {
    const a = await call(answer({ ok: true, data: { schemaVersion: 1, kind: "list", data: {} } }), { op: "list" }, "list");
    expect(a.exitCode).toBe(0);
  });

  it.each([
    ["Specified native messaging host not found.", "not-installed"],
    ["Access to the specified native messaging host is forbidden.", "forbidden"],
    ["No such native application io.github.roystondwork_dev.portcheck", "not-installed"],
    ["This extension does not have permission to use native application io.github.roystondwork_dev.portcheck (or the application is not installed)", "forbidden"],
    ["Native host has exited.", "failed"],
  ])("explains %j", async (message, problem) => {
    const err = await call(failing(message), { op: "version" }, "version").catch((e: unknown) => e);
    expect(err).toBeInstanceOf(HostError);
    expect((err as HostError).problem).toBe(problem);
    expect((err as HostError).name).toBe("HostError");
  });

  it("handles non-Error rejections", async () => {
    // eslint-disable-next-line @typescript-eslint/prefer-promise-reject-errors -- testing a non-Error rejection
    const send: SendNative = () => Promise.reject("boom");
    await expect(call(send, { op: "version" }, "version")).rejects.toThrow("portcheck couldn't be started: boom");
  });

  it("reports portcheck's error", async () => {
    await expect(call(answer({ ok: false, exitCode: 1, error: "refusing to kill launchd (pid 1)" }), { op: "kill", pid: 1 }, "kill")).rejects.toThrow(
      "refusing to kill launchd (pid 1)",
    );
    await expect(call(answer({ ok: false, error: "" }), { op: "list" }, "list")).rejects.toThrow("portcheck sent no answer.");
    await expect(call(answer(undefined), { op: "list" }, "list")).rejects.toThrow("portcheck sent no answer.");
    await expect(call(answer({ ok: true, data: "x" }), { op: "list" }, "list")).rejects.toThrow("portcheck sent no answer.");
  });

  it("rejects other schema versions and kinds", async () => {
    await expect(call(answer({ ok: true, data: { schemaVersion: 2, kind: "list", data: {} } }), { op: "list" }, "list")).rejects.toThrow(
      "schema 2 isn't supported",
    );
    await expect(call(answer({ ok: true, data: { schemaVersion: 1, kind: "check", data: {} } }), { op: "list" }, "list")).rejects.toThrow(
      'expected "list" from portcheck, got "check"',
    );
  });
});
