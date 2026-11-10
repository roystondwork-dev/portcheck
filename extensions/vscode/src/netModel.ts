// Turns `portcheck net`, `net devices` and `net lookup` output into quick pick
// entries. Pure: no "vscode" import, so it is unit-tested in isolation.

import type { DevicesData, LookupData, NetData } from "./schema";

export interface NetItem {
  label: string;
  description?: string;
  detail?: string;
  /** What choosing the item copies; undefined for items that run an action. */
  copy?: string;
  /** For items that run an action instead of copying. */
  action?: "public" | "probe";
}

/** Strips the prefix length: "192.168.1.10/24" → "192.168.1.10". */
export function bareAddress(cidr: string): string {
  return cidr.split("/")[0] as string;
}

/**
 * This machine's addresses, most useful first: the primary IPv4 and IPv6,
 * then the other addresses of physical and VPN interfaces (not loopback or
 * link-local, which another device can't use), then the gateway and DNS.
 */
export function ipItems(data: NetData): NetItem[] {
  const items: NetItem[] = [];
  const seen = new Set<string>();
  const primary = data.primary ?? {};
  const add = (address: string, description: string, detail?: string) => {
    if (seen.has(address)) {
      return;
    }
    seen.add(address);
    items.push({
      label: address,
      description,
      ...(detail === undefined ? {} : { detail }),
      copy: address,
    });
  };
  if (primary.ipv4 !== undefined) {
    add(
      primary.ipv4,
      `primary IPv4${primary.interface ? ` · ${primary.interface}` : ""}`,
      "Other devices on your network reach this machine here",
    );
  }
  if (primary.ipv6 !== undefined) {
    add(
      primary.ipv6,
      `primary IPv6${primary.interface ? ` · ${primary.interface}` : ""}`,
    );
  }
  for (const ni of data.interfaces) {
    if (ni.kind === "loopback") {
      continue;
    }
    for (const a of ni.addresses) {
      if (a.class === "link-local" || a.class === "loopback") {
        continue;
      }
      add(
        bareAddress(a.address),
        `${ni.name} · ${a.class}${ni.kind === "physical" ? "" : ` · ${ni.kind}`}`,
      );
    }
  }
  for (const g of data.gateways ?? []) {
    add(g, "gateway (your router)");
  }
  for (const d of data.dns ?? []) {
    add(d, "DNS server");
  }
  for (const p of data.public ?? []) {
    add(p, "public address", "How the internet sees you (from ipify.org)");
  }
  if (data.public === undefined) {
    items.push({
      label: "$(globe) Show My Public Address",
      detail: "Asks ipify.org; the only part that contacts the internet",
      action: "public",
    });
  }
  return items;
}

/** Devices on the network: the gateway first, then by address. */
export function deviceItems(data: DevicesData): NetItem[] {
  const items = [...data.devices]
    .sort(
      (a, b) =>
        Number(b.gateway === true) - Number(a.gateway === true) ||
        compareIP(a.ip, b.ip),
    )
    .map((d) => {
      const what = [
        d.gateway ? "gateway (your router)" : "",
        d.self ? "this machine" : "",
        d.name ?? "",
        d.vendor ?? "",
      ].filter((x) => x !== "");
      const item: NetItem = { label: d.ip, copy: d.ip };
      if (what.length > 0) {
        item.description = what.join(" · ");
      }
      const detail = [d.mac, d.interface]
        .filter((x) => x !== undefined)
        .join(" · ");
      if (detail !== "") {
        item.detail = detail;
      }
      return item;
    });
  if (data.probed === undefined) {
    items.push({
      label: "$(search) Probe for More Devices",
      detail:
        "Sends connection attempts to the devices on your local network, to find ones not seen yet",
      action: "probe",
    });
  }
  return items;
}

/** One entry per record, grouped by query; failed queries say why. */
export function lookupItems(data: LookupData): NetItem[] {
  const items: NetItem[] = [];
  for (const r of data.results) {
    if (r.error !== undefined) {
      items.push({
        label: r.query,
        description: "doesn't resolve",
        detail: r.error,
      });
      continue;
    }
    for (const rec of r.records) {
      items.push({
        label: rec.value,
        description: `${rec.type}${rec.note ? ` · ${rec.note}` : ""}`,
        detail: r.query,
        copy: rec.value,
      });
    }
  }
  return items;
}

/** Splits user input into lookup queries: spaces or commas. */
export function lookupQueries(input: string): string[] {
  return input
    .split(/[\s,]+/)
    .map((q) => q.trim())
    .filter((q) => q !== "");
}

/** Orders IPv4 numerically and puts IPv6 after IPv4. */
export function compareIP(a: string, b: string): number {
  const v4 = (ip: string) =>
    /^\d+\.\d+\.\d+\.\d+$/.test(ip) ? ip.split(".").map(Number) : undefined;
  const x = v4(a);
  const y = v4(b);
  if (x && y) {
    for (let i = 0; i < 4; i++) {
      if (x[i] !== y[i]) {
        return (x[i] as number) - (y[i] as number);
      }
    }
    return 0;
  }
  if (x) {
    return -1;
  }
  if (y) {
    return 1;
  }
  return a.localeCompare(b);
}
