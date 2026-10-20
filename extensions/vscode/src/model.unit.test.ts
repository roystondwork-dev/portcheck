import { readFileSync } from "node:fs";
import * as path from "node:path";
import { describe, expect, it } from "vitest";
import { ancestry, buildRows, containerLabel, validatePort } from "./model";
import type { Envelope, ListData, Socket } from "./schema";

const goldenList = (JSON.parse(
  readFileSync(path.join(__dirname, "..", "..", "..", "internal", "output", "testdata", "list.json.golden"), "utf8"),
) as Envelope<ListData>).data;

describe("buildRows", () => {
  it("turns real list output into sorted rows", () => {
    const rows = buildRows(goldenList);
    expect(rows.map((r) => r.label)).toEqual([
      ":22  (owner hidden)",
      ":3000  node", // the LISTEN and ESTABLISHED sockets of one owner share a row
      ":5353  ?",
      ":5432  postgres",
      ":18080  docker:web",
      ":50000  (owner hidden)",
    ]);
    const hidden = rows[0];
    expect(hidden?.kind).toBe("hidden");
    expect(hidden?.contextValue).toBe("port.hidden.tcp");
    expect(hidden?.tooltip).toContain("owner is hidden");
    expect(hidden?.pid).toBeUndefined();

    const web = rows.find((r) => r.kind === "container");
    expect(web?.contextValue).toBe("port.container.tcp");
    expect(web?.tooltip).toContain("Stop with: `docker stop web`");
    expect(web?.tooltip).toContain("Container: **docker:web** (nginx:alpine)");
    expect(web?.container?.stopHint).toBe("docker stop web");

    const node = rows.find((r) => r.processName === "node");
    expect(node?.ancestry).toBe("npm (4200) ← zsh (900)");
    expect(node?.tooltip).toContain("Started by: npm (4200) ← zsh (900)");
    expect(node?.tooltip).toContain("Command: `node --max-old-space-size");
    expect(node?.tooltip).toContain("user alice");
    expect(node?.description).toBe("tcp · 127.0.0.1 · pid 4242");

    const udp = rows.find((r) => r.port === 5353);
    expect(udp?.contextValue).toBe("port.process.udp");
  });

  it("merges IPv4 and IPv6 listeners of one owner", () => {
    const sock = (addr: string, pid?: number): Socket => ({
      proto: "tcp",
      localAddress: addr,
      localPort: 8080,
      state: "LISTEN",
      ...(pid === undefined ? {} : { pid, process: { pid, name: "java" } }),
    });
    const rows = buildRows({ sockets: [sock("::", 7), sock("0.0.0.0", 7), sock("0.0.0.0", 8)], partial: false });
    expect(rows).toHaveLength(2);
    expect(rows[0]?.description).toBe("tcp · 0.0.0.0, :: · pid 7");
    expect(rows[0]?.tooltip).not.toContain("Started by");
    expect(rows[0]?.ancestry).toBeUndefined();
    expect(rows[0]?.tooltip).toContain("Process: **java** (pid 7)");
    expect(rows[0]?.tooltip).not.toContain("user");
  });

  it("handles unknown process names and processes without details", () => {
    const rows = buildRows({
      sockets: [{ proto: "tcp", localAddress: "127.0.0.1", localPort: 1, state: "LISTEN", pid: 9 }],
      partial: false,
    });
    expect(rows[0]?.label).toBe(":1  ?");
    expect(rows[0]?.processName).toBeUndefined();
    expect(rows[0]?.tooltip).toContain("owner is hidden");
  });

  it("labels WSL ports and containers without image", () => {
    const rows = buildRows({
      sockets: [
        {
          proto: "tcp",
          localAddress: "127.0.0.1",
          localPort: 3000,
          state: "LISTEN",
          pid: 15872,
          process: { pid: 15872, name: "wslrelay.exe" },
          container: { name: "Ubuntu", runtime: "wsl", process: "node", processPid: 12, stopHint: "wsl -d Ubuntu --exec kill 12" },
        },
      ],
      partial: false,
    });
    expect(rows[0]?.label).toBe(":3000  wsl:Ubuntu/node");
    expect(rows[0]?.tooltip).toContain("Container: **wsl:Ubuntu/node**\n");
    expect(rows[0]?.tooltip).toContain("wslrelay.exe");
  });

  it("shows a container whose proxy process is hidden (Linux docker-proxy runs as root)", () => {
    const rows = buildRows({
      sockets: [
        {
          proto: "tcp",
          localAddress: "0.0.0.0",
          localPort: 8080,
          state: "LISTEN",
          container: { id: "abc", name: "api", image: "api:dev", runtime: "docker", stopHint: "docker stop api" },
        },
      ],
      partial: true,
    });
    expect(rows[0]?.kind).toBe("container");
    expect(rows[0]?.label).toBe(":8080  docker:api");
    expect(rows[0]?.tooltip).not.toContain("hidden");
    expect(rows[0]?.description).toBe("tcp · 0.0.0.0");
  });

  it("returns no rows for no sockets", () => {
    expect(buildRows({ sockets: [], partial: false })).toEqual([]);
  });
});

describe("helpers", () => {
  it("containerLabel falls back from name to image to id", () => {
    expect(containerLabel({ name: "web", runtime: "docker", stopHint: "" })).toBe("docker:web");
    expect(containerLabel({ image: "redis", runtime: "podman", stopHint: "" })).toBe("podman:redis");
    expect(containerLabel({ id: "abc", runtime: "docker", stopHint: "" })).toBe("docker:abc");
    expect(containerLabel({ runtime: "docker", stopHint: "" })).toBe("docker:?");
  });

  it("ancestry", () => {
    expect(ancestry(undefined)).toBe("");
    expect(ancestry({ pid: 1, name: "x" })).toBe("");
    expect(ancestry({ pid: 1, name: "x", ancestors: [{ pid: 2, name: "a" }, { pid: 3, name: "b" }] })).toBe("a (2) ← b (3)");
  });

  it("validatePort", () => {
    for (const ok of ["1", " 3000 ", "65535"]) {
      expect(validatePort(ok)).toBeUndefined();
    }
    for (const bad of ["", "0", "65536", "-1", "3.5", "http"]) {
      expect(validatePort(bad)).toBe("Enter a port between 1 and 65535");
    }
  });
});
