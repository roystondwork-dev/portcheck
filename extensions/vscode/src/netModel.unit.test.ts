import { describe, expect, it } from "vitest";
import {
  bareAddress,
  compareIP,
  deviceItems,
  ipItems,
  lookupItems,
  lookupQueries,
} from "./netModel";
import type { NetData } from "./schema";

const net: NetData = {
  interfaces: [
    {
      name: "lo0",
      kind: "loopback",
      addresses: [
        { address: "127.0.0.1/8", class: "loopback", version: "IPv4" },
      ],
    },
    {
      name: "en0",
      mac: "00:00:5e:00:53:0a",
      kind: "physical",
      addresses: [
        { address: "192.168.1.10/24", class: "private", version: "IPv4" },
        { address: "2001:db8::10/64", class: "public", version: "IPv6" },
        { address: "2001:db8::11/64", class: "public", version: "IPv6" },
        { address: "fe80::1/64", class: "link-local", version: "IPv6" },
      ],
    },
    {
      name: "utun3",
      kind: "vpn",
      addresses: [
        { address: "100.64.0.5/32", class: "shared", version: "IPv4" },
      ],
    },
    {
      name: "x0",
      kind: "virtual",
      addresses: [{ address: "::1/128", class: "loopback", version: "IPv6" }],
    },
  ],
  primary: { ipv4: "192.168.1.10", ipv6: "2001:db8::10", interface: "en0" },
  gateways: ["192.168.1.1"],
  dns: ["192.168.1.1", "192.0.2.53"],
};

describe("ipItems", () => {
  it("lists the primary addresses first, then usable ones, gateway and DNS", () => {
    const items = ipItems(net);
    expect(items.map((i) => `${i.label} | ${i.description ?? ""}`)).toEqual([
      "192.168.1.10 | primary IPv4 · en0",
      "2001:db8::10 | primary IPv6 · en0",
      "2001:db8::11 | en0 · public",
      "100.64.0.5 | utun3 · shared · vpn",
      "192.168.1.1 | gateway (your router)",
      "192.0.2.53 | DNS server",
      "$(globe) Show My Public Address | ",
    ]);
    expect(items[0]?.detail).toContain("Other devices");
    expect(items[0]?.copy).toBe("192.168.1.10");
    expect(items.at(-1)).toMatchObject({ action: "public" });
  });
  it("shows the public address once fetched, and copes with little data", () => {
    const items = ipItems({ ...net, public: ["203.0.113.7"] });
    expect(items.at(-1)).toMatchObject({
      label: "203.0.113.7",
      description: "public address",
      copy: "203.0.113.7",
    });
    expect(items.some((i) => i.action === "public")).toBe(false);
    const bare = ipItems({ interfaces: [], primary: { ipv4: "10.0.0.2" } });
    expect(bare[0]).toMatchObject({
      label: "10.0.0.2",
      description: "primary IPv4",
    });
    expect(
      ipItems({ interfaces: [], primary: { ipv6: "2001:db8::2" } })[0]
        ?.description,
    ).toBe("primary IPv6");
    expect(ipItems({ interfaces: [] }).map((i) => i.action)).toEqual([
      "public",
    ]);
  });
});

describe("deviceItems", () => {
  it("puts the gateway first, then sorts by address, and offers a probe", () => {
    const items = deviceItems({
      devices: [
        {
          ip: "192.168.1.20",
          mac: "b8:27:eb:00:00:20",
          vendor: "Raspberry Pi",
          interface: "en0",
        },
        {
          ip: "192.168.1.10",
          mac: "00:00:5e:00:53:0a",
          interface: "en0",
          self: true,
        },
        {
          ip: "192.168.1.1",
          mac: "00:00:5e:00:53:01",
          name: "router.lan",
          interface: "en0",
          gateway: true,
        },
        { ip: "192.168.1.9" },
      ],
    });
    expect(
      items.map(
        (i) => `${i.label} | ${i.description ?? ""} | ${i.detail ?? ""}`,
      ),
    ).toEqual([
      "192.168.1.1 | gateway (your router) · router.lan | 00:00:5e:00:53:01 · en0",
      "192.168.1.9 |  | ",
      "192.168.1.10 | this machine | 00:00:5e:00:53:0a · en0",
      "192.168.1.20 | Raspberry Pi | b8:27:eb:00:00:20 · en0",
      "$(search) Probe for More Devices |  | Sends connection attempts to the devices on your local network, to find ones not seen yet",
    ]);
    expect(deviceItems({ devices: [], probed: "192.168.1.0/24" })).toEqual([]);
  });
});

describe("lookupItems", () => {
  it("lists records per query and failures", () => {
    const items = lookupItems({
      results: [
        {
          query: "example.com",
          kind: "domain",
          records: [
            { type: "A", value: "192.0.2.80", note: "public" },
            { type: "NS", value: "ns.example.com" },
          ],
        },
        {
          query: "nope.invalid",
          kind: "domain",
          records: [],
          error: "nope.invalid doesn't resolve",
        },
      ],
    });
    expect(items).toEqual([
      {
        label: "192.0.2.80",
        description: "A · public",
        detail: "example.com",
        copy: "192.0.2.80",
      },
      {
        label: "ns.example.com",
        description: "NS",
        detail: "example.com",
        copy: "ns.example.com",
      },
      {
        label: "nope.invalid",
        description: "doesn't resolve",
        detail: "nope.invalid doesn't resolve",
      },
    ]);
  });
});

describe("helpers", () => {
  it("splits queries, strips prefixes and orders addresses", () => {
    expect(lookupQueries(" example.com, 1.1.1.1  github.com ")).toEqual([
      "example.com",
      "1.1.1.1",
      "github.com",
    ]);
    expect(lookupQueries("  ")).toEqual([]);
    expect(bareAddress("192.168.1.10/24")).toBe("192.168.1.10");
    expect(bareAddress("192.168.1.10")).toBe("192.168.1.10");
    expect(
      ["10.0.0.10", "2001:db8::1", "10.0.0.9", "10.0.0.9", "::1"].sort(
        compareIP,
      ),
    ).toEqual(["10.0.0.9", "10.0.0.9", "10.0.0.10", "::1", "2001:db8::1"]);
    expect(compareIP("::1", "10.0.0.1")).toBe(1);
  });
});
