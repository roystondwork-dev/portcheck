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
