// A thin client for the portcheck CLI's JSON output (ADR-0004). No port logic
// lives here: every call runs `portcheck … --json` and validates the result.

import * as path from "node:path";
import {
  type CheckData,
  type DoctorData,
  type Envelope,
  type FreeData,
  type KillData,
  type ListData,
  SCHEMA_VERSION,
  type VersionData,
} from "./schema";

export interface ExecResult {
  stdout: string;
  stderr: string;
  code: number;
}

/** Runs a program; resolves with its exit code instead of rejecting on non-zero exit. */
export type ExecFn = (file: string, args: readonly string[], timeoutMs: number) => Promise<ExecResult>;

export class PortcheckError extends Error {
  constructor(
    message: string,
    readonly exitCode?: number,
    /** portcheck's full error output, without the usage hint. */
    readonly detail = "",
  ) {
    super(message);
    this.name = "PortcheckError";
  }
}

export interface ResolveOptions {
  /** The portcheck.path setting; empty means unset. */
  configured: string;
  /** Directory of the installed extension. */
  extensionPath: string;
  platform: NodeJS.Platform;
  exists: (file: string) => boolean;
}

/** Picks the binary: the setting, else the one bundled in bin/, else portcheck on PATH. */
export function resolveBinary(o: ResolveOptions): string {
  if (o.configured.trim() !== "") {
    return o.configured.trim();
  }
  const exe = o.platform === "win32" ? "portcheck.exe" : "portcheck";
  const bundled = path.join(o.extensionPath, "bin", exe);
  return o.exists(bundled) ? bundled : exe;
}

const DEFAULT_TIMEOUT_MS = 15_000;

export class PortcheckClient {
  constructor(
    private readonly binary: string,
    private readonly exec: ExecFn,
    private readonly timeoutMs = DEFAULT_TIMEOUT_MS,
  ) {}

  /**
   * Runs portcheck with --json. Exit codes in okCodes are expected outcomes
   * (e.g. 1 = "port in use" for check). Output must be a schema v1 envelope
   * of the expected kind.
   */
  async run<T>(kind: string, args: readonly string[], okCodes: readonly number[] = [0]): Promise<T> {
    const r = await this.exec(this.binary, [...args, "--json"], this.timeoutMs);
    const out = r.stdout.trim();
    if (!okCodes.includes(r.code) || out === "") {
      throw new PortcheckError(firstLine(r.stderr) ?? `portcheck exited with status ${r.code}`, r.code, errorDetail(r.stderr));
    }
    let env: Envelope<T>;
    try {
      env = JSON.parse(out) as Envelope<T>;
    } catch {
      throw new PortcheckError(`portcheck printed invalid JSON for "${kind}"`, r.code);
    }
    if (env.schemaVersion !== SCHEMA_VERSION) {
      throw new PortcheckError(
        `portcheck JSON schema ${env.schemaVersion} is not supported (expected ${SCHEMA_VERSION}); update portcheck or the extension`,
        r.code,
      );
    }
    if (env.kind !== kind) {
      throw new PortcheckError(`expected "${kind}" output from portcheck, got "${env.kind}"`, r.code);
    }
    return env.data;
  }

  list(): Promise<ListData> {
    return this.run<ListData>("list", ["list", "--listening"]);
  }

  check(port: number, proto: "tcp" | "udp" = "tcp"): Promise<CheckData> {
    return this.run<CheckData>("check", ["check", String(port), "--proto", proto], [0, 1]);
  }

  free(range: string, count = 1): Promise<FreeData> {
    return this.run<FreeData>("free", ["free", range, "-n", String(count)], [0, 1]);
  }

  /** Stops a process. Exit codes 1, 3 and 4 still carry per-process results. */
  kill(pid: number, force = false): Promise<KillData> {
    const args = ["kill", "--pid", String(pid), "--yes"];
    if (force) {
      args.push("--force");
    }
    return this.run<KillData>("kill", args, [0, 1, 3, 4]);
  }

  /** Checks a project's .portcheck.yaml. Exit code 2 (missing or invalid file) is an error. */
  doctor(config: string): Promise<DoctorData> {
    return this.run<DoctorData>("doctor", ["doctor", "--config", config], [0, 1]);
  }

  version(): Promise<VersionData> {
    return this.run<VersionData>("version", ["version"]);
  }
}

function errorDetail(stderr: string): string {
  return stderr
    .split("\n")
    .filter((l) => !l.startsWith("Run 'portcheck"))
    .join("\n")
    .trim()
    .replace(/^portcheck: /, "");
}

function firstLine(s: string): string | undefined {
  const line = s
    .split("\n")
    .map((l) => l.trim())
    .find((l) => l !== "");
  return line?.replace(/^portcheck: /, "");
}
