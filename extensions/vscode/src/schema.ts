// Types for portcheck's JSON output, schema version 1.
// Source of truth: docs/json-schema.md in the portcheck repository.

export const SCHEMA_VERSION = 1;

export interface Envelope<T> {
  schemaVersion: number;
  kind: string;
  data: T;
}

export interface ProcessRef {
  pid: number;
  name: string;
}

export interface ProcessInfo {
  pid: number;
  name: string;
  user?: string;
  cmdline?: string;
  exe?: string;
  startTime?: string;
  ancestors?: ProcessRef[];
  /** The process's working directory. */
  cwd?: string;
  /** The project it runs in, e.g. "my-shop" (portcheck 0.11+). */
  project?: string;
}

export interface ContainerInfo {
  id?: string;
  name?: string;
  image?: string;
  runtime: string;
  process?: string;
  processPid?: number;
  stopHint: string;
}

export interface Socket {
  proto: "tcp" | "udp";
  localAddress: string;
  localPort: number;
  remoteAddress?: string;
  remotePort?: number;
  state: string;
  pid?: number;
  process?: ProcessInfo;
  container?: ContainerInfo;
}

export interface ListData {
  sockets: Socket[];
  partial: boolean;
}

export interface PortStatus {
  port: number;
  proto: string;
  status: "free" | "in-use" | "denied";
  holders: Socket[];
}

export interface CheckData {
  ports: PortStatus[];
}

export interface FreeData {
  ports: number[];
}

export interface KillResult {
  pid: number;
  name?: string;
  outcome: "terminated" | "killed" | "already-exited" | "dry-run" | "failed";
  error?: string;
}

export interface KillData {
  results: KillResult[];
}

export interface VersionData {
  version: string;
  commit: string;
  platform: string;
}

export interface DoctorCheck {
  name: string;
  port: number;
  proto: string;
  expect: "free" | "open";
  host?: string;
  ok: boolean;
  status: string;
  detail?: string;
  fix?: string;
}

export interface DoctorData {
  config: string;
  ok: boolean;
  checks: DoctorCheck[];
}

/** `portcheck net` (portcheck 0.12+). */
export interface NetData {
  interfaces: {
    name: string;
    mac?: string;
    mtu?: number;
    kind: "loopback" | "vpn" | "virtual" | "physical";
    addresses: { address: string; class: string; version: "IPv4" | "IPv6" }[];
  }[];
  primary?: { ipv4?: string; ipv6?: string; interface?: string };
  gateways?: string[];
  dns?: string[];
  public?: string[];
  warnings?: string[];
}

/** `portcheck net devices`. */
export interface DevicesData {
  devices: {
    ip: string;
    mac?: string;
    vendor?: string;
    name?: string;
    interface?: string;
    gateway?: boolean;
    self?: boolean;
  }[];
  probed?: string;
  warnings?: string[];
}

/** `portcheck net lookup`. */
export interface LookupData {
  results: {
    query: string;
    kind: "domain" | "ip";
    records: { type: string; value: string; note?: string }[];
    error?: string;
  }[];
}
