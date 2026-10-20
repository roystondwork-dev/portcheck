import { type ExecFileException, execFile } from "node:child_process";
import { type ExecFn, type ExecResult, PortcheckError } from "./cli";

/** ExecFn backed by child_process.execFile. */
export const nodeExec: ExecFn = (file, args, timeoutMs) =>
  new Promise((resolve, reject) => {
    execFile(file, [...args], { timeout: timeoutMs, maxBuffer: 32 * 1024 * 1024, windowsHide: true }, (err, stdout, stderr) => {
      const r = settle(err, stdout, stderr, file, timeoutMs);
      if (r instanceof PortcheckError) {
        reject(r);
      } else {
        resolve(r);
      }
    });
  });

/**
 * Maps an execFile outcome to a result (any exit code) or an error (the
 * program couldn't run). Separate from nodeExec so every case is testable on
 * every OS.
 */
export function settle(
  err: ExecFileException | null,
  stdout: string,
  stderr: string,
  file: string,
  timeoutMs: number,
): ExecResult | PortcheckError {
  if (err === null) {
    return { stdout, stderr, code: 0 };
  }
  if (typeof err.code === "number") {
    return { stdout, stderr, code: err.code };
  }
  if (err.code === "ENOENT") {
    return new PortcheckError(`portcheck was not found ("${file}"). Install it or set "portcheck.path".`);
  }
  return new PortcheckError(err.killed ? `portcheck timed out after ${timeoutMs} ms` : err.message);
}
