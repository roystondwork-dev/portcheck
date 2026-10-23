import * as http from "node:http";
import type { AddressInfo } from "node:net";
import { describe, expect, it } from "vitest";
import { isUp } from "./probe";

const realFetch = (url: string, init: RequestInit) => fetch(url, init);

describe("isUp", () => {
  it("is true when a server answers, even with an error status", async () => {
    const server = http.createServer((_, res) => res.writeHead(500).end());
    await new Promise<void>((r) => server.listen(0, "127.0.0.1", r));
    const { port } = server.address() as AddressInfo;
    try {
      expect(await isUp(realFetch, `http://127.0.0.1:${port}/`)).toBe(true);
    } finally {
      server.close();
    }
  });

  it("is false when nothing listens", async () => {
    const server = http.createServer();
    await new Promise<void>((r) => server.listen(0, "127.0.0.1", r));
    const { port } = server.address() as AddressInfo;
    await new Promise((r) => server.close(r));
    expect(await isUp(realFetch, `http://127.0.0.1:${port}/`)).toBe(false);
  });

  it("is false when the server doesn't answer in time", async () => {
    const never = (_: string, init: RequestInit) =>
      new Promise((_resolve, reject) => init.signal?.addEventListener("abort", () => { reject(new Error("aborted")); }));
    expect(await isUp(never, "http://localhost:1/", 20)).toBe(false);
  });
});
