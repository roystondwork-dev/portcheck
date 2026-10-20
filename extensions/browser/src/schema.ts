// Types for portcheck's JSON output (schema version 1) used by the extension.
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
  ancestors?: ProcessRef[];
}

export interface ContainerInfo {
  id?: string;
  name?: string;
  image?: string;
  runtime: string;
  process?: string;
  stopHint: string;
}

export interface Socket {
  proto: "tcp" | "udp";
  localAddress: string;
  localPort: number;
  state: string;
  pid?: number;
  process?: ProcessInfo;
  container?: ContainerInfo;
}

export interface ListData {
  sockets: Socket[];
  partial: boolean;
}

export interface CheckData {
  ports: { port: number; proto: string; status: "free" | "in-use" | "denied"; holders: Socket[] }[];
}

export interface KillData {
  results: { pid: number; name?: string; outcome: string; error?: string }[];
}

export interface VersionData {
  version: string;
  platform: string;
}
