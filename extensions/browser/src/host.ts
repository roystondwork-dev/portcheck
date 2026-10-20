// Talks to portcheck through native messaging (ADR-0005). portcheck answers
// with the same JSON its --json commands print.
import { type Envelope, SCHEMA_VERSION } from "./schema";

/** The name portcheck registers with `portcheck browser install`. */
export const HOST = "io.github.roystondwork_dev.portcheck";

export type Request =
  | { op: "version" }
  | { op: "list" }
  | { op: "check"; port: number }
  | { op: "kill"; pid: number; force?: boolean };

/** chrome.runtime.sendNativeMessage, injectable for tests. */
export type SendNative = (host: string, message: object) => Promise<unknown>;

/** Why portcheck couldn't answer. */
export type HostProblem =
  | "not-installed" // the browser has no registration for portcheck
  | "forbidden" // registered, but not for this extension's ID
  | "failed"; // portcheck ran but reported an error

export class HostError extends Error {
  constructor(
    message: string,
    readonly problem: HostProblem,
  ) {
    super(message);
    this.name = "HostError";
  }
}

export interface Answer<T> {
  data: T;
  /** portcheck's exit code, e.g. 1 when a checked port is in use. */
  exitCode: number;
}

interface Response {
  ok?: unknown;
  exitCode?: unknown;
  data?: unknown;
  error?: unknown;
}

/** Sends a request and returns the data of the expected kind. */
export async function call<T>(send: SendNative, req: Request, kind: string): Promise<Answer<T>> {
  let raw: unknown;
  try {
    raw = await send(HOST, req);
  } catch (err) {
    const msg = err instanceof Error ? err.message : String(err);
    if (/not found/i.test(msg)) {
      throw new HostError("portcheck isn't set up for this browser.", "not-installed");
    }
    if (/forbidden/i.test(msg)) {
      throw new HostError("portcheck is set up, but not for this extension.", "forbidden");
    }
    throw new HostError(`portcheck couldn't be started: ${msg}`, "failed");
  }
  const resp = (raw ?? {}) as Response;
  const error = typeof resp.error === "string" && resp.error !== "" ? resp.error : undefined;
  const env = resp.data as Envelope<T> | undefined;
  if (resp.ok !== true || typeof env !== "object") {
    throw new HostError(error ?? "portcheck sent no answer.", "failed");
  }
  if (env.schemaVersion !== SCHEMA_VERSION) {
    throw new HostError(`portcheck's JSON schema ${env.schemaVersion} isn't supported; update portcheck or the extension.`, "failed");
  }
  if (env.kind !== kind) {
    throw new HostError(`expected "${kind}" from portcheck, got "${env.kind}".`, "failed");
  }
  return { data: env.data, exitCode: typeof resp.exitCode === "number" ? resp.exitCode : 0 };
}
