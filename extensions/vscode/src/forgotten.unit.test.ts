import { describe, expect, it } from "vitest";
import { forgottenMessage, forgottenServers, HOUR } from "./forgotten";
import type { ListData, Socket } from "./schema";

const now = Date.parse("2026-11-09T12:00:00Z");
const ago = (h: number) => new Date(now - h * HOUR).toISOString();

function sock(
  port: number,
  pid: number,
  project: string | undefined,
  startedHoursAgo: number | undefined,
): Socket {
  return {
    proto: "tcp",
    localAddress: "127.0.0.1",
    localPort: port,
    state: "LISTEN",
    pid,
    process: {
      pid,
      name: pid === 11 ? "python3" : "node",
      ...(project === undefined ? {} : { project }),
      ...(startedHoursAgo === undefined
        ? {}
        : { startTime: ago(startedHoursAgo) }),
    },
  };
}

describe("forgottenServers", () => {
  it("keeps old processes in a project, one per process, oldest first", () => {
    const data: ListData = {
      partial: false,
      sockets: [
        sock(5173, 10, "shop", 50),
        { ...sock(5173, 10, "shop", 50), localAddress: "::1" },
        sock(24678, 10, "shop", 50),
        sock(8000, 11, "api", 80),
        sock(4000, 12, "docs", 2), // too new
        sock(9000, 13, undefined, 100), // no project
        sock(9001, 14, "x", undefined), // no start time
        {
          ...sock(9002, 15, "x", 100),
          process: {
            pid: 15,
            name: "n",
            project: "x",
            startTime: "not a date",
          },
        },
        {
          ...sock(8080, 16, "shop", 100),
          container: {
            runtime: "docker",
            name: "web",
            stopHint: "docker stop web",
          },
        },
      ],
    };
    expect(forgottenServers(data, now, 24 * HOUR)).toEqual([
      {
        pid: 11,
        name: "python3",
        project: "api",
        ports: [8000],
        started: now - 80 * HOUR,
      },
      {
        pid: 10,
        name: "node",
        project: "shop",
        ports: [5173, 24678],
        started: now - 50 * HOUR,
      },
    ]);
    const noProcess: Socket = {
      proto: "tcp",
      localAddress: "0.0.0.0",
      localPort: 22,
      state: "LISTEN",
    };
    expect(
      forgottenServers({ partial: false, sockets: [noProcess] }, now, HOUR),
    ).toEqual([]);
  });
});

describe("forgottenMessage", () => {
  const f = (n: number) =>
    Array.from({ length: n }, (_, i) => ({
      pid: i,
      name: "node",
      project: `p${i}`,
      ports: [3000 + i],
      started: 0,
    }));
  it("describes one, several and many servers", () => {
    expect(forgottenMessage(f(1), 24 * HOUR)).toBe(
      "1 dev server has been running for over a day: node in p0 (:3000).",
    );
    expect(forgottenMessage(f(2), 48 * HOUR)).toBe(
      "2 dev servers have been running for over 2 days: node in p0 (:3000), node in p1 (:3001).",
    );
    expect(forgottenMessage(f(5), 6 * HOUR)).toBe(
      "5 dev servers have been running for over 6 hours: node in p0 (:3000), node in p1 (:3001), node in p2 (:3002), and 2 more.",
    );
    expect(forgottenMessage(f(1), 0)).toContain("over 1 hours");
  });
});
