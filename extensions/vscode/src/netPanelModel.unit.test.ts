import { describe, expect, it } from "vitest";
import {
  addressesResult,
  checkResult,
  devicesResult,
  errorResult,
  isLocal,
  lookupResult,
  markChanges,
  MAX_PORTS,
  type MonitorMemory,
  monitorsResult,
  parseMonitorAdd,
  parseHost,
  parseLookup,
  parsePorts,
  parseRequest,
  portSpec,
  projectMissingResult,
  projectResult,
  type Result,
  type Row,
  rowStates,
  scanResult,
  trackMonitors,
  watchInterval,
} from "./netPanelModel";
import type { CheckData, ScanData } from "./schema";

describe("parseRequest", () => {
  it("accepts each request the webview sends", () => {
    expect(parseRequest({ type: "check", host: "h", ports: "1" })).toEqual({
      type: "check",
      host: "h",
      ports: "1",
    });
    expect(parseRequest({ type: "lookup", text: "a.com" })).toEqual({
      type: "lookup",
      text: "a.com",
    });
    expect(parseRequest({ type: "addresses", public: true })).toEqual({
      type: "addresses",
      public: true,
    });
    expect(parseRequest({ type: "addresses" })).toEqual({
      type: "addresses",
      public: false,
    });
    expect(parseRequest({ type: "devices", probe: true })).toEqual({
      type: "devices",
      probe: true,
    });
    expect(parseRequest({ type: "devices", probe: "yes" })).toEqual({
      type: "devices",
      probe: false,
    });
    expect(parseRequest({ type: "copy", text: "10.0.0.1" })).toEqual({
      type: "copy",
      text: "10.0.0.1",
    });
    expect(parseRequest({ type: "stop", port: 3000 })).toEqual({
      type: "stop",
      port: 3000,
    });
  });

  it("drops anything malformed", () => {
    for (const m of [
      null,
      "check",
      42,
      {},
      { type: "nope" },
      { type: "check", host: "h" },
      { type: "check", ports: "1" },
      { type: "check", host: 1, ports: "1" },
      { type: "lookup" },
      { type: "copy" },
      { type: "copy", text: "" },
      { type: "stop", port: "3000" },
      { type: "stop", port: 0 },
      { type: "stop", port: 65536 },
      { type: "stop", port: 1.5 },
    ]) {
      expect(parseRequest(m), JSON.stringify(m)).toBeUndefined();
    }
  });
});

describe("parsePorts", () => {
  it("reads single ports, lists and ranges, sorted and without duplicates", () => {
    expect(parsePorts("3000")).toEqual([3000]);
    expect(parsePorts(" 443, 80 80\t8000-8002 ")).toEqual([
      80, 443, 8000, 8001, 8002,
    ]);
    expect(
      parsePorts("1-65535".replace("65535", String(MAX_PORTS))),
    ).toHaveLength(MAX_PORTS);
  });

  it("explains what's wrong", () => {
    expect(parsePorts("  ")).toMatch(/Enter a port/);
    expect(parsePorts("http")).toBe('"http" is not a port or a range.');
    expect(parsePorts("0")).toMatch(/1 to 65535; "0"/);
    expect(parsePorts("70000")).toMatch(/1 to 65535/);
    expect(parsePorts("10-70000")).toMatch(/1 to 65535/);
    expect(parsePorts("90-80")).toMatch(/backwards/);
    expect(parsePorts(`1-${MAX_PORTS + 1}`)).toMatch(/more than 1024/);
    expect(parsePorts(`1-${MAX_PORTS},5000`)).toMatch(/more than 1024/);
  });
});

describe("parseHost and isLocal", () => {
  it("defaults to localhost and refuses ranges and options", () => {
    expect(parseHost("")).toBe("localhost");
    expect(parseHost("  db.internal ")).toBe("db.internal");
    expect(parseHost("10.0.0.0/24")).toHaveProperty("error");
    expect(parseHost("a b")).toHaveProperty("error");
    expect(parseHost("-p")).toHaveProperty("error");
  });

  it("knows which hosts are this machine", () => {
    for (const h of [
      "localhost",
      "LOCALHOST",
      "app.localhost",
      "127.0.0.1",
      "127.1.2.3",
      "::1",
      "[::1]",
      "0.0.0.0",
      "::",
    ]) {
      expect(isLocal(h), h).toBe(true);
    }
    for (const h of [
      "db.internal",
      "192.168.1.20",
      "128.0.0.1",
      "localhost.com",
    ]) {
      expect(isLocal(h), h).toBe(false);
    }
  });
});

describe("portSpec", () => {
  it("compresses runs into ranges", () => {
    expect(portSpec([])).toBe("");
    expect(portSpec([80])).toBe("80");
    expect(portSpec([80, 443, 8000, 8001, 8002, 9000])).toBe(
      "80,443,8000-8002,9000",
    );
  });
});

describe("checkResult", () => {
  const data: CheckData = {
    ports: [
      { port: 3000, proto: "tcp", status: "free", holders: [] },
      {
        port: 5173,
        proto: "tcp",
        status: "in-use",
        holders: [
          {
            proto: "tcp",
            localAddress: "127.0.0.1",
            localPort: 5173,
            state: "LISTEN",
            pid: 42,
            process: { pid: 42, name: "node", project: "my-shop" },
          },
          {
            proto: "tcp",
            localAddress: "::1",
            localPort: 5173,
            state: "LISTEN",
            pid: 43,
            process: { pid: 43, name: "vite" },
          },
        ],
      },
      {
        port: 8080,
        proto: "tcp",
        status: "in-use",
        holders: [
          {
            proto: "tcp",
            localAddress: "0.0.0.0",
            localPort: 8080,
            state: "LISTEN",
            container: {
              runtime: "docker",
              name: "web",
              stopHint: "docker stop web",
            },
          },
          {
            proto: "tcp",
            localAddress: "::",
            localPort: 8080,
            state: "LISTEN",
            container: {
              runtime: "podman",
              image: "nginx",
              stopHint: "podman stop x",
            },
          },
          {
            proto: "tcp",
            localAddress: "::",
            localPort: 8080,
            state: "LISTEN",
            container: { runtime: "docker", stopHint: "docker stop y" },
          },
        ],
      },
      { port: 22, proto: "tcp", status: "in-use", holders: [] },
      {
        port: 25,
        proto: "tcp",
        status: "in-use",
        holders: [
          {
            proto: "tcp",
            localAddress: "0.0.0.0",
            localPort: 25,
            state: "LISTEN",
          },
        ],
      },
      { port: 631, proto: "tcp", status: "denied", holders: [] },
    ],
  };

  it("shows who holds each port, with Copy and Stop where they make sense", () => {
    const r = checkResult(data);
    expect(r.section).toBe("ports");
    expect(r.summary).toBe("6 ports on this machine: 4 in use.");
    expect(r.rows).toEqual([
      { cells: ["3000/tcp", "free", "Nothing is listening"], tone: "ok" },
      {
        cells: ["5173/tcp", "in use", "node (pid 42) · in my-shop"],
        tone: "bad",
        copy: "42",
        stopPort: 5173,
      },
      {
        cells: ["5173/tcp", "in use", "vite (pid 43)"],
        tone: "bad",
        copy: "43",
        stopPort: 5173,
      },
      {
        cells: [
          "8080/tcp",
          "in use",
          "docker container web. Stop it with: docker stop web",
        ],
        tone: "bad",
        copy: "docker stop web",
      },
      {
        cells: [
          "8080/tcp",
          "in use",
          "podman container nginx. Stop it with: podman stop x",
        ],
        tone: "bad",
        copy: "podman stop x",
      },
      {
        cells: [
          "8080/tcp",
          "in use",
          "docker container a container. Stop it with: docker stop y",
        ],
        tone: "bad",
        copy: "docker stop y",
      },
      {
        cells: ["22/tcp", "in use", "by a process hidden from your user"],
        tone: "bad",
      },
      {
        cells: ["25/tcp", "in use", "by a process hidden from your user"],
        tone: "bad",
      },
      {
        cells: ["631/tcp", "unknown", "Not allowed to check this port"],
        tone: "warn",
      },
    ]);
  });

  it("says when everything is free", () => {
    expect(
      checkResult({
        ports: [{ port: 3000, proto: "tcp", status: "free", holders: [] }],
      }).summary,
    ).toBe("1 port on this machine: all free.");
  });
});

describe("scanResult", () => {
  it("shows each port's state, service and latency", () => {
    const data: ScanData = {
      results: [
        {
          host: "db.internal",
          address: "10.0.0.5",
          port: 5432,
          state: "open",
          service: "postgresql",
          latencyMs: 1.234,
        },
        {
          host: "10.0.0.5",
          address: "10.0.0.5",
          port: 6379,
          state: "closed",
          latencyMs: 2,
        },
        { host: "h", address: "", port: 81, state: "filtered" },
        {
          host: "h",
          address: "",
          port: 82,
          state: "error",
          error: "no route to host",
        },
        { host: "h", address: "", port: 83, state: "open" },
      ],
      summary: {
        hosts: 1,
        ports: 5,
        open: 2,
        closed: 1,
        filtered: 1,
        errors: 1,
        durationMs: 12,
      },
    };
    const r = scanResult(data);
    expect(r.columns).toEqual(["Port", "Status", "Details", "Host"]);
    expect(r.rows).toEqual([
      {
        cells: [
          "5432",
          "open",
          "Accepts connections · postgresql · 1.2 ms",
          "db.internal (10.0.0.5)",
        ],
        tone: "ok",
      },
      {
        cells: [
          "6379",
          "closed",
          "The host refused the connection",
          "10.0.0.5",
        ],
        tone: "bad",
      },
      {
        cells: ["81", "filtered", "No answer in time (often a firewall)", "h"],
        tone: "warn",
      },
      { cells: ["82", "error", "no route to host", "h"], tone: "bad" },
      { cells: ["83", "open", "Accepts connections", "h"], tone: "ok" },
    ]);
    expect(r.summary).toBe(
      "5 ports checked in 12 ms: 2 open, 1 closed, 1 filtered, 1 failed.",
    );
    expect(
      scanResult({
        results: [],
        summary: { ...data.summary, ports: 1, errors: 0 },
      }).summary,
    ).toBe("1 port checked in 12 ms: 2 open, 1 closed, 1 filtered.");
  });
});

describe("lookupResult", () => {
  it("lists records, and names that don't resolve", () => {
    const r = lookupResult({
      results: [
        {
          query: "example.com",
          kind: "domain",
          records: [
            { type: "A", value: "192.0.2.10" },
            {
              type: "CNAME",
              value: "edge.example.net",
              note: "canonical name",
            },
          ],
        },
        {
          query: "nope.invalid",
          kind: "domain",
          records: [],
          error: "no such host",
        },
      ],
    });
    expect(r.rows).toEqual([
      { cells: ["example.com", "A", "192.0.2.10"], copy: "192.0.2.10" },
      {
        cells: ["example.com", "CNAME · canonical name", "edge.example.net"],
        copy: "edge.example.net",
      },
      {
        cells: ["nope.invalid", "doesn't resolve", "no such host"],
        tone: "bad",
      },
    ]);
  });
});

describe("addressesResult", () => {
  it("lists addresses to copy, primary first, without the public-address action", () => {
    const r = addressesResult({
      interfaces: [
        {
          name: "en0",
          kind: "physical",
          addresses: [
            { address: "192.168.1.10/24", class: "private", version: "IPv4" },
          ],
        },
      ],
      primary: { ipv4: "192.168.1.10", interface: "en0" },
      gateways: ["192.168.1.1"],
    });
    expect(r.rows).toEqual([
      {
        cells: ["192.168.1.10", "primary IPv4 · en0"],
        copy: "192.168.1.10",
        tone: "info",
      },
      { cells: ["192.168.1.1", "gateway (your router)"], copy: "192.168.1.1" },
    ]);
    expect(r.summary).toMatch(/Read locally/);
    expect(
      addressesResult({ interfaces: [], public: ["203.0.113.7"] }).summary,
    ).toMatch(/ipify/);
  });
});

describe("devicesResult", () => {
  it("lists devices to copy, without the probe action", () => {
    const seen = devicesResult({
      devices: [
        { ip: "192.168.1.1", gateway: true, mac: "aa:bb:cc:00:00:01" },
        { ip: "192.168.1.20" },
      ],
    });
    expect(seen.rows).toEqual([
      {
        cells: ["192.168.1.1", "gateway (your router)", "aa:bb:cc:00:00:01"],
        copy: "192.168.1.1",
      },
      { cells: ["192.168.1.20", "", ""], copy: "192.168.1.20" },
    ]);
    expect(seen.summary).toBe(
      "2 devices your machine has talked to recently. Probe to find more.",
    );
    expect(
      devicesResult({ devices: [{ ip: "10.0.0.2" }], probed: "10.0.0.0/24" })
        .summary,
    ).toBe("1 device on 10.0.0.0/24.");
  });
});

describe("errorResult and parseLookup", () => {
  it("builds an error result", () => {
    expect(errorResult("lookup", "boom")).toEqual({
      type: "result",
      section: "lookup",
      columns: [],
      rows: [],
      error: "boom",
    });
  });

  it("splits lookups, or asks for input", () => {
    expect(parseLookup("a.com, 1.1.1.1")).toEqual(["a.com", "1.1.1.1"]);
    expect(parseLookup(" ")).toMatch(/Enter a domain/);
  });
});

describe("watching", () => {
  it("parses watch, unwatch and project requests", () => {
    expect(parseRequest({ type: "watch", host: "h", ports: "1" })).toEqual({
      type: "watch",
      host: "h",
      ports: "1",
    });
    expect(parseRequest({ type: "watch", host: "h" })).toBeUndefined();
    expect(parseRequest({ type: "unwatch" })).toEqual({ type: "unwatch" });
    expect(parseRequest({ type: "project" })).toEqual({ type: "project" });
  });

  it("checks this machine often and other hosts gently", () => {
    expect(watchInterval("localhost")).toBe(2000);
    expect(watchInterval("db.internal")).toBe(5000);
  });

  const local = (rows: Row[]): Result => ({
    type: "result",
    section: "ports",
    columns: [],
    rows,
  });

  it("notes nothing on the first check", () => {
    const r = local([{ cells: ["3000/tcp", "free", "Nothing is listening"] }]);
    expect(markChanges(undefined, r, "localhost")).toEqual([]);
    expect(r.rows[0]?.changed).toBeUndefined();
  });

  it("marks and describes what changed", () => {
    const before = rowStates(
      [
        { cells: ["3000/tcp", "in use", "node (pid 1)"] },
        { cells: ["3000/tcp", "in use", "node (pid 2)"] },
        { cells: ["4000/tcp", "free", "Nothing is listening"] },
        { cells: ["5000/tcp", "in use", "python (pid 7)"] },
      ],
      "localhost",
    );
    expect(before.get("3000/tcp")).toBe("in use\tnode (pid 1)");
    const now = local([
      { cells: ["3000/tcp", "free", "Nothing is listening"] },
      { cells: ["4000/tcp", "free", "Nothing is listening"] },
      { cells: ["5000/tcp", "in use", "vite (pid 9)"] },
      { cells: ["5000/tcp", "in use", "vite (pid 10)"] },
      { cells: ["6000/tcp", "free", "Nothing is listening"] },
    ]);
    expect(markChanges(before, now, "localhost")).toEqual([
      "3000/tcp is now free",
      "5000/tcp is now held by vite (pid 9)",
    ]);
    expect(now.rows.map((r) => r.changed === true)).toEqual([
      true,
      false,
      true,
      true,
      false,
    ]);
  });

  it("names the host for other machines", () => {
    const before = rowStates(
      [
        { cells: ["80", "open", "Accepts · 40.1 ms", "h"] },
        { cells: ["443", "closed", "refused", "h"] },
      ],
      "db.internal",
    );
    // A new latency on 80 is not a change; 443 opening is.
    const now = local([
      { cells: ["80", "open", "Accepts · 52.7 ms", "h"] },
      { cells: ["443", "open", "Accepts · 41.0 ms", "h"] },
    ]);
    expect(markChanges(before, now, "db.internal")).toEqual([
      "db.internal 443 is now open",
    ]);
    expect(now.rows.map((r) => r.changed === true)).toEqual([false, true]);
  });
});

describe("project ports", () => {
  it("lists each check, with Stop and Copy where they help", () => {
    const r = projectResult({
      config: "/w/.portcheck.yaml",
      ok: false,
      checks: [
        {
          name: "web",
          port: 3000,
          proto: "tcp",
          expect: "free",
          ok: true,
          status: "free",
        },
        {
          name: "api",
          port: 8080,
          proto: "tcp",
          expect: "free",
          ok: false,
          status: "in-use",
          detail: "in use by node (pid 4)",
          fix: "portcheck kill 8080",
        },
        {
          name: "db",
          port: 5432,
          proto: "tcp",
          expect: "open",
          host: "db.internal",
          ok: false,
          status: "closed",
        },
        {
          name: "cache",
          port: 6379,
          proto: "tcp",
          expect: "open",
          ok: true,
          status: "open",
          detail: "accepts connections",
          fix: "unused",
        },
      ],
    });
    expect(r.section).toBe("project");
    expect(r.summary).toBe("Project ports: 2 ok, 2 need attention.");
    expect(r.rows).toEqual([
      { cells: ["web", "3000/tcp", "ok", "should be free"], tone: "ok" },
      {
        cells: [
          "api",
          "8080/tcp",
          "needs attention",
          "should be free: in use by node (pid 4). portcheck kill 8080",
        ],
        tone: "bad",
        stopPort: 8080,
        copy: "portcheck kill 8080",
      },
      {
        cells: [
          "db",
          "5432/tcp on db.internal",
          "needs attention",
          "should be open: closed",
        ],
        tone: "bad",
      },
      {
        cells: [
          "cache",
          "6379/tcp",
          "ok",
          "should be open: accepts connections",
        ],
        tone: "ok",
      },
    ]);
  });

  it("says when everything is ok", () => {
    expect(
      projectResult({
        config: "c",
        ok: true,
        checks: [
          {
            name: "web",
            port: 1,
            proto: "tcp",
            expect: "free",
            ok: true,
            status: "free",
          },
        ],
      }).summary,
    ).toBe("Project ports: all 1 ok.");
  });

  it("explains a missing or broken file", () => {
    expect(projectMissingResult(false)).toMatchObject({
      section: "project",
      rows: [],
      summary: expect.stringMatching(/No .portcheck.yaml/) as unknown,
    });
    expect(projectMissingResult(true).error).toMatch(/Problems/);
  });
});

describe("monitors", () => {
  it("parses monitor requests and refuses odd URLs to open", () => {
    expect(parseRequest({ type: "monitors" })).toEqual({ type: "monitors" });
    expect(parseRequest({ type: "monitorAdd", target: "https://a/" })).toEqual({
      type: "monitorAdd",
      target: "https://a/",
      every: "",
      contains: "",
    });
    expect(
      parseRequest({
        type: "monitorAdd",
        target: "a:1",
        every: "5m",
        contains: "x",
      }),
    ).toEqual({
      type: "monitorAdd",
      target: "a:1",
      every: "5m",
      contains: "x",
    });
    expect(parseRequest({ type: "monitorAdd" })).toBeUndefined();
    expect(parseRequest({ type: "monitorRemove", name: "site" })).toEqual({
      type: "monitorRemove",
      name: "site",
    });
    expect(parseRequest({ type: "monitorRemove", name: "" })).toBeUndefined();
    expect(parseRequest({ type: "open", url: "https://a.example/x" })).toEqual({
      type: "open",
      url: "https://a.example/x",
    });
    for (const url of [
      "file:///etc/passwd",
      "javascript:alert(1)",
      "https://a b",
      3,
    ]) {
      expect(parseRequest({ type: "open", url })).toBeUndefined();
    }
  });

  it("checks what's added", () => {
    expect(parseMonitorAdd(" https://a/health ", "1m", " pong ")).toEqual({
      target: "https://a/health",
      every: "1m",
      contains: "pong",
    });
    expect(parseMonitorAdd("db.internal:5432", "5m", "")).toEqual({
      target: "db.internal:5432",
      every: "5m",
      contains: "",
    });
    expect(parseMonitorAdd("[::1]:80", "30s", "")).toMatchObject({
      target: "[::1]:80",
    });
    expect(parseMonitorAdd("3000", "15m", "")).toMatchObject({
      target: "3000",
    });
    expect(parseMonitorAdd("  ", "1m", "")).toMatch(/Enter a URL/);
    expect(parseMonitorAdd("a b", "1m", "")).toMatch(/one URL/);
    expect(parseMonitorAdd("--every", "1m", "")).toMatch(/one URL/);
    expect(parseMonitorAdd("ftp://a/", "1m", "")).toMatch(/http:\/\/ or https/);
    expect(parseMonitorAdd("example.com", "1m", "")).toMatch(
      /http:\/\/ or https/,
    );
    expect(parseMonitorAdd("https://a/", "2s", "")).toMatch(/how often/);
    expect(parseMonitorAdd("a:1", "1m", "x")).toMatch(/only works for URLs/);
  });

  const up = (name: string) => ({
    name,
    endpoint: "e",
    up: true,
    latencyMs: 1,
    at: "",
  });
  const down = (name: string, error?: string) => ({
    name,
    endpoint: "e",
    up: false,
    latencyMs: 1,
    at: "",
    ...(error === undefined ? {} : { error }),
  });

  it("warns once after two failures, and once when it's back", () => {
    let mem = new Map<string, MonitorMemory>();
    const step = (results: ReturnType<typeof up>[], now: number) => {
      const r = trackMonitors(mem, { results }, now);
      mem = r.next;
      return r.messages;
    };
    expect(step([up("site"), down("db", "refused")], 0)).toEqual([]);
    expect(
      step([down("site", "timed out"), down("db", "refused")], 60_000),
    ).toEqual(["db is down: refused"]);
    expect(step([down("site"), down("db", "refused")], 120_000)).toEqual([
      "site is down: no answer",
    ]);
    expect(step([up("site"), up("db")], 30 * 60_000)).toEqual([
      "site is back up after 28 minutes",
      "db is back up after 29 minutes",
    ]);
    expect(step([down("site")], 31 * 60_000)).toEqual([]);
    expect(mem.has("db")).toBe(false); // no longer checked: forgotten
    mem = new Map([["x", { fails: 2, down: true, since: 0 }]]);
    expect(step([up("x")], 20_000)).toEqual([
      "x is back up after less than a minute",
    ]);
    mem = new Map([["x", { fails: 2, down: true, since: 0 }]]);
    expect(step([up("x")], 60_000)).toEqual(["x is back up after 1 minute"]);
  });

  it("shows each monitor with its last check", () => {
    const list = {
      file: "/c/monitors.yaml",
      monitors: [
        { name: "site", url: "https://a/" },
        { name: "db", address: "db:5432" },
        { name: "api", url: "https://api/" },
        { name: "cache", address: "cache:6379" },
        { name: "new", url: "https://new/" },
      ],
    };
    const check = {
      results: [
        { ...up("site"), status: 200, latencyMs: 84.4 },
        { ...up("db"), latencyMs: 1.2 },
        down("api", "HTTP 503 Service Unavailable"),
        down("cache"),
      ],
    };
    const memory = new Map<string, MonitorMemory>([
      ["api", { fails: 2, down: true, since: 0 }],
      ["cache", { fails: 1, down: false, since: 0 }],
    ]);
    const r = monitorsResult(list, check, memory);
    expect(r.summary).toBe(
      "5 monitors: 1 down. Checked every minute while this panel is open.",
    );
    expect(r.rows).toEqual([
      {
        cells: ["site", "up", "HTTP 200 · 84 ms", "https://a/"],
        tone: "ok",
        copy: "https://a/",
        remove: "site",
        open: "https://a/",
      },
      {
        cells: ["db", "up", "accepts connections · 1 ms", "db:5432"],
        tone: "ok",
        copy: "db:5432",
        remove: "db",
      },
      {
        cells: ["api", "down", "HTTP 503 Service Unavailable", "https://api/"],
        tone: "bad",
        copy: "https://api/",
        remove: "api",
        open: "https://api/",
      },
      {
        cells: ["cache", "failing", "no answer", "cache:6379"],
        tone: "warn",
        copy: "cache:6379",
        remove: "cache",
      },
      {
        cells: ["new", "checking", "", "https://new/"],
        copy: "https://new/",
        remove: "new",
        open: "https://new/",
      },
    ]);
    // Without memory (first round), a failure counts as down.
    expect(
      monitorsResult(
        { file: "f", monitors: [{ name: "x", address: "x:1" }] },
        { results: [down("x")] },
        new Map(),
      ).rows[0]?.cells[1],
    ).toBe("down");
    expect(
      monitorsResult(
        { file: "f", monitors: [{ name: "w" }] },
        { results: [up("w")] },
        new Map(),
      ).summary,
    ).toBe(
      "1 monitor: none down. Checked every minute while this panel is open.",
    );
    const empty = monitorsResult(
      { file: "f", monitors: [] },
      { results: [] },
      new Map(),
    );
    expect(empty.rows).toEqual([]);
    expect(empty.summary).toMatch(/No monitors yet/);
  });
});
