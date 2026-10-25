/** The data of `portcheck check --json` (schema v1). */
export interface CheckData {
  ports: { port: number; proto: string; status: "free" | "in-use" | "denied"; holders: unknown[] }[];
}

/** The portcheck binary: PORTCHECK_BIN, the cached one, or a download of this package's version. */
export function binary(): Promise<string>;
/** Runs `portcheck check <port> --json`; null if portcheck can't be run or doesn't answer in time. */
export function check(port: number, bin: string): Promise<CheckData | null>;
/** Who holds the port, as lines to print, and the processes `kill` may stop. */
export function describe(
  port: number,
  data: CheckData | null,
): { lines: string[]; targets: { pid: number; name: string }[] };
/** Stops one process with portcheck kill's guards; true if it was stopped. */
export function kill(pid: number, bin: string): Promise<boolean>;
/** Downloads (once) and returns the portcheck binary for this OS and CPU. */
export function ensureBinary(opts: { version: string; checksums: Record<string, string> }): Promise<string>;
