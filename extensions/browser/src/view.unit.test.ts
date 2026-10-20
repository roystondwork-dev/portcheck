import { describe, expect, it } from "vitest";
import { checkGolden } from "./fixtures";
import { portState, type PortState } from "./model";
import { filterPorts, portView, setupHelp, uptime } from "./view";

const NOW = Date.parse("2026-10-20T09:42:00Z"); // 12 minutes after the golden node started

describe("uptime", () => {
  it.each([
    ["2026-10-20T09:41:30Z", "just now"],
    ["2026-10-20T09:30:00Z", "12 min"],
    ["2026-10-20T06:30:00Z", "3 h"],
    ["2026-10-17T09:00:00Z", "3 d"],
  ])("%s → %s", (start, want) => {
    expect(uptime(start, NOW)).toBe(want);
  });
  it("ignores missing, invalid and future times", () => {
    expect(uptime(undefined, NOW)).toBeUndefined();
    expect(uptime("yesterday", NOW)).toBeUndefined();
    expect(uptime("2026-10-21T00:00:00Z", NOW)).toBeUndefined();
  });
});

describe("portView", () => {
  it("shows a running process with who started it (real check output)", () => {
    const v = portView("localhost:3000", portState(checkGolden.data, 3000), NOW);
    expect(v).toMatchObject({ address: "localhost:3000", status: "running", chip: "Running", summary: "node · pid 4242" });
    expect(v.details).toEqual([
      { icon: "tree", text: "Started by npm (4200) ← zsh (900)" },
      { icon: "terminal", text: "node --max-old-space-size=4096 ./node_modules/.bin/next dev --turbo --hostname 127.0.0.1", code: true },
      { icon: "user", text: "alice · 12 min" },
    ]);
    expect(v.actions).toEqual([
      { kind: "stop", label: "Stop process", style: "danger", pid: 4242 },
      { kind: "copy", label: "Copy PID", style: "secondary", text: "4242" },
    ]);
  });

  it("explains not running, no access and hidden", () => {
    expect(portView("localhost:3001", portState(checkGolden.data, 3001), NOW)).toMatchObject({ status: "stopped", chip: "Not running", actions: [] });
    expect(portView("localhost:80", portState(checkGolden.data, 80), NOW)).toMatchObject({ status: "denied", chip: "No access", details: [] });
    expect(portView("localhost:22", portState(checkGolden.data, 22), NOW)).toMatchObject({
      status: "hidden",
      actions: [{ kind: "copy", label: "Copy command", style: "secondary", text: "sudo portcheck kill 22" }],
    });
  });

  it("offers a container's stop command, never a kill, and lists other holders", () => {
    const s: PortState = {
      port: 8080,
      status: "in-use",
      holders: [
        { name: "docker:web", pid: 1, ancestry: "", hidden: false, container: { name: "web", image: "nginx:alpine", runtime: "docker", stopHint: "docker stop web" } },
        { name: "java", pid: 9, ancestry: "", hidden: false, startTime: "2026-10-20T09:41:59Z" },
        { name: "docker:db", ancestry: "", hidden: false, container: { name: "db", runtime: "docker", stopHint: "docker stop db" } },
      ],
    };
    const v = portView("localhost:8080", s, NOW);
    expect(v.summary).toBe("docker:web");
    expect(v.details).toEqual([
      { icon: "box", text: "docker:web (nginx:alpine)" },
      { icon: "plus", text: "Also java (pid 9)" },
      { icon: "plus", text: "Also docker:db" },
    ]);
    expect(v.actions).toEqual([{ kind: "copy", label: "Copy stop command", style: "secondary", text: "docker stop web" }]);
  });

  it("shows uptime without a user, and a container without image", () => {
    const s: PortState = {
      port: 1,
      status: "in-use",
      holders: [{ name: "docker:x", ancestry: "", hidden: false, startTime: "2026-10-20T09:00:00Z", container: { name: "x", runtime: "docker", stopHint: "docker stop x" } }],
    };
    expect(portView("localhost:1", s, NOW).details).toEqual([
      { icon: "box", text: "docker:x" },
      { icon: "clock", text: "42 min" },
    ]);
  });
});

describe("setupHelp", () => {
  it("guides installing and connecting portcheck", () => {
    const h = setupHelp("not-installed", "");
    expect(h.headline).toBe("Connect portcheck");
    expect(h.steps.map((s) => s.command)).toEqual([
      "brew install roystondwork-dev/tap/portcheck",
      "scoop bucket add portcheck https://github.com/roystondwork-dev/scoop-bucket; scoop install portcheck",
      "portcheck browser install",
    ]);
    expect(setupHelp("forbidden", "").steps).toEqual([{ text: "Update portcheck, then run:", command: "portcheck browser install" }]);
    expect(setupHelp("failed", "boom")).toEqual({ headline: "portcheck reported a problem", intro: "boom", steps: [] });
  });
});

describe("filterPorts", () => {
  const rows = [
    { port: 22, owners: "(hidden)", url: "u22" },
    { port: 3000, owners: "node", url: "u3000" },
    { port: 5432, owners: "postgres", url: "u5432" },
  ];
  it("puts the current tab's port first", () => {
    expect(filterPorts(rows, "", 5432).map((r) => [r.port, r.current])).toEqual([[5432, true], [22, false], [3000, false]]);
  });
  it("filters by port or owner", () => {
    expect(filterPorts(rows, " 30 ", undefined).map((r) => r.port)).toEqual([3000]);
    expect(filterPorts(rows, "POST", undefined).map((r) => r.port)).toEqual([5432]);
    expect(filterPorts(rows, "nothing", undefined)).toEqual([]);
  });
});
