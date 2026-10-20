import { execFile } from "node:child_process";
import { type ExecFn, PortcheckError } from "./cli";

/** ExecFn backed by child_process.execFile. */
export const nodeExec: ExecFn = (file, args, timeoutMs) =>
  new Promise((resolve, reject) => {
    execFile(file, [...args], { timeout: timeoutMs, maxBuffer: 32 * 1024 * 1024, windowsHide: true }, (err, stdout, stderr) => {
      if (err === null) {
        resolve({ stdout, stderr, code: 0 });
        return;
      }
      if (typeof err.code === "number") {
        resolve({ stdout, stderr, code: err.code });
        return;
      }
      if (err.code === "ENOENT") {
        reject(new PortcheckError(`portcheck was not found ("${file}"). Install it or set "portcheck.path".`));
        return;
      }
      reject(new PortcheckError(err.killed ? `portcheck timed out after ${timeoutMs} ms` : err.message));
    });
  });
