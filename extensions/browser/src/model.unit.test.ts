import { describe, expect, it } from "vitest";
import { checkGolden, listGolden } from "./fixtures";
import { ancestry, badge, containerLabel, listedPorts, portState } from "./model";
import type { CheckData, Socket } from "./schema";

const sock = (o: Partial<Socket>): Socket => ({ proto: "tcp", localAddress: "127.0.0.1", localPort: 3000, state: "LISTEN", ...o });

describe("portState and badge (real check output)", () => {
  it("names the process and who started it", () => {
    const s = portState(checkGolden.data, 3000);
    expect(s.status).toBe("in-use");
    expect(s.holders[0]).toMatchObject({ name: "node", pid: 4242, user: "alice", ancestry: "npm (4200) ← zsh (900)", hidden: false });
    expect(badge("localhost:3000", s)).toEqual({
      text: "node",
      color: "#2E7D32",
      title: "localhost:3000 is served by node (pid 4242), started by npm (4200) ← zsh (900).",
    });
  });

  it("says when nothing listens", () => {
    expect(badge("localhost:3001", portState(checkGolden.data, 3001))).toMatchObject({ text: "off", color: "#C62828" });
    // A port missing from the answer counts as free.
    expect(portState(checkGolden.data, 9).status).toBe("free");
  });

  it("explains denied and hidden owners", () => {
    expect(badge("localhost:80", portState(checkGolden.data, 80))).toMatchObject({ text: "!" });
    expect(badge("localhost:22", portState(checkGolden.data, 22))).toMatchObject({ text: "?", title: "localhost:22 is served by a process hidden from your user." });
  });
});

describe("portState", () => {
  const data = (holders: Socket[]): CheckData => ({ ports: [{ port: 3000, proto: "tcp", status: "in-use", holders }] });

  it("merges IPv4/IPv6 sockets of one process and counts others", () => {
    const s = portState(data([sock({ pid: 7, process: { pid: 7, name: "java" } }), sock({ localAddress: "::", pid: 7, process: { pid: 7, name: "java" } }), sock({ pid: 8, process: { pid: 8, name: "java" } })]), 3000);
    expect(s.holders).toHaveLength(2);
    expect(badge("localhost:3000", s).title).toBe("localhost:3000 is served by java (pid 7) and 1 more.");
  });

  it("labels containers and processes without details", () => {
    const web = portState(data([sock({ pid: 1, container: { name: "web", runtime: "docker", stopHint: "docker stop web" } })]), 3000);
    expect(web.holders[0]).toMatchObject({ name: "docker:web", hidden: false });
    expect(badge("localhost:3000", web)).toMatchObject({ text: "web", title: "localhost:3000 is served by docker:web." });

    const img = portState(data([sock({ container: { image: "redis:7", runtime: "podman", stopHint: "podman stop x" } })]), 3000);
    expect(badge("localhost:3000", img).text).toBe("redi");
    const anon = portState(data([sock({ container: { id: "abc", runtime: "docker", stopHint: "docker stop abc" } })]), 3000);
    expect(badge("localhost:3000", anon).text).toBe("ctr");

    const nameless = portState(data([sock({ pid: 5 })]), 3000);
    expect(nameless.holders[0]).toMatchObject({ name: "?", pid: 5, ancestry: "" });
    const exe = portState(data([sock({ pid: 6, process: { pid: 6, name: "python.exe" } })]), 3000);
    expect(badge("localhost:3000", exe)).toMatchObject({ text: "pyth", title: "localhost:3000 is served by python.exe (pid 6)." });
  });

  it("treats an in-use port without holders as hidden", () => {
    expect(badge("x", portState(data([]), 3000)).text).toBe("?");
  });
});

describe("helpers", () => {
  it("containerLabel and ancestry", () => {
    expect(containerLabel({ name: "u", runtime: "wsl", process: "node", stopHint: "" })).toBe("wsl:u/node");
    expect(containerLabel({ runtime: "docker", stopHint: "" })).toBe("docker:?");
    expect(ancestry(undefined)).toBe("");
  });
});

describe("listedPorts (real list output)", () => {
  it("lists listening TCP ports with owners, once per port", () => {
    expect(listedPorts(listGolden.data)).toEqual([
      { port: 22, owners: "(hidden)", url: "http://localhost:22/" },
      { port: 3000, owners: "node", url: "http://localhost:3000/" },
      { port: 5432, owners: "postgres", url: "http://localhost:5432/" },
      { port: 18080, owners: "docker:web", url: "http://localhost:18080/" },
      { port: 50000, owners: "(hidden)", url: "http://localhost:50000/" },
    ]);
  });
});
