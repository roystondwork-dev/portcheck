import { describe, expect, it } from "vitest";
import { checkGolden } from "./fixtures";
import { portState, type PortState } from "./model";
import { portView, setupHelp } from "./view";

describe("portView", () => {
  it("shows the process, who started it, and Stop (real check output)", () => {
    const v = portView("localhost:3000", portState(checkGolden.data, 3000));
    expect(v.headline).toBe("localhost:3000 is served by node (pid 4242)");
    expect(v.tone).toBe("good");
    expect(v.lines.map((l) => l.label)).toEqual(["Started by", "Command", "User"]);
    expect(v.lines[1]).toMatchObject({ code: true });
    expect(v.actions).toEqual([{ kind: "stop", label: "Stop node", pid: 4242 }]);
  });

  it("explains free, denied and hidden", () => {
    expect(portView("localhost:3001", portState(checkGolden.data, 3001))).toMatchObject({ headline: "Nothing is listening on localhost:3001", tone: "bad", actions: [] });
    expect(portView("localhost:80", portState(checkGolden.data, 80))).toMatchObject({ tone: "muted", actions: [] });
    expect(portView("localhost:22", portState(checkGolden.data, 22)).actions).toEqual([{ kind: "copy", label: "Copy command", text: "sudo portcheck kill 22" }]);
  });

  it("offers the container's stop command instead of killing its proxy, and lists others", () => {
    const s: PortState = {
      port: 8080,
      status: "in-use",
      holders: [
        { name: "docker:web", pid: 1, ancestry: "", hidden: false, container: { name: "web", runtime: "docker", stopHint: "docker stop web" } },
        { name: "java", pid: 9, ancestry: "", hidden: false },
        { name: "docker:db", ancestry: "", hidden: false, container: { name: "db", runtime: "docker", stopHint: "docker stop db" } },
      ],
    };
    const v = portView("localhost:8080", s);
    expect(v.headline).toBe("localhost:8080 is served by docker:web");
    expect(v.lines).toEqual([
      { label: "Stop with", value: "docker stop web", code: true },
      { label: "Also", value: "java (pid 9)" },
      { label: "Also", value: "docker:db" },
    ]);
    expect(v.actions).toEqual([{ kind: "copy", label: "Copy stop command", text: "docker stop web" }]);
  });
});

describe("setupHelp", () => {
  it("guides installing and connecting portcheck", () => {
    const h = setupHelp("not-installed", "");
    expect(h.headline).toBe("Connect portcheck to this browser");
    expect(h.steps.map((s) => s.command)).toEqual([
      "brew install roystondwork-dev/tap/portcheck",
      "scoop bucket add portcheck https://github.com/roystondwork-dev/scoop-bucket; scoop install portcheck",
      "portcheck browser install",
    ]);
    expect(setupHelp("forbidden", "").steps).toEqual([{ text: "Update portcheck, then run:", command: "portcheck browser install" }]);
    expect(setupHelp("failed", "boom")).toEqual({ headline: "portcheck reported a problem", steps: [{ text: "boom" }] });
  });
});
