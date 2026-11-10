import * as vscode from "vscode";
import type { Deps } from "./commands";
import {
  deviceItems,
  ipItems,
  lookupItems,
  lookupQueries,
  type NetItem,
} from "./netModel";

type Pickable = vscode.QuickPickItem & NetItem;

/** Shows the items; copies the chosen one, or returns its action. */
async function pickAndCopy(
  d: Deps,
  items: NetItem[],
  placeHolder: string,
): Promise<NetItem | undefined> {
  const chosen = await d.ui().pick<Pickable>(items, placeHolder);
  if (chosen?.copy !== undefined) {
    await vscode.env.clipboard.writeText(chosen.copy);
    void d.ui().info(`Copied ${chosen.copy}.`);
  }
  return chosen;
}

function warn(d: Deps, warnings: string[] | undefined): void {
  for (const w of warnings ?? []) {
    d.log.warn(`net: ${w}`);
  }
}

/** portcheck: Show My IP Addresses. Returns the address copied, if any. */
export async function showIP(d: Deps): Promise<string | undefined> {
  let data = await d.client().net();
  warn(d, data.warnings);
  let chosen = await pickAndCopy(
    d,
    ipItems(data),
    "Your addresses: choose one to copy it",
  );
  if (chosen?.action === "public") {
    data = await d.client().net(true);
    warn(d, data.warnings);
    chosen = await pickAndCopy(
      d,
      ipItems(data),
      "Your addresses, with the public one: choose one to copy it",
    );
  }
  return chosen?.copy;
}

/** portcheck: Devices on My Network. Returns the address copied, if any. */
export async function networkDevices(d: Deps): Promise<string | undefined> {
  let data = await d.client().devices();
  warn(d, data.warnings);
  let chosen = await pickAndCopy(
    d,
    deviceItems(data),
    "Devices on your network: choose one to copy its address",
  );
  if (chosen?.action === "probe") {
    const ok = await d
      .ui()
      .confirm(
        "Probe your local network for more devices?",
        "portcheck sends connection attempts to every address on your local network (only private networks, rate-limited). Only probe networks you own or are allowed to test.",
        "Probe",
      );
    if (ok !== "Probe") {
      return undefined;
    }
    data = await d.client().devices(true);
    warn(d, data.warnings);
    chosen = await pickAndCopy(
      d,
      deviceItems(data),
      `Devices on ${data.probed ?? "your network"}: choose one to copy its address`,
    );
  }
  return chosen?.copy;
}

/** portcheck: Look Up a Domain or IP. Returns the value copied, if any. */
export async function lookup(
  d: Deps,
  input?: string,
): Promise<string | undefined> {
  const text =
    input ??
    (await d.ui().input({
      prompt: "Domain names or IP addresses to look up",
      placeHolder: "example.com 192.168.1.20",
      validateInput: (v) =>
        lookupQueries(v).length === 0
          ? "Enter a domain name or an IP address"
          : undefined,
    }));
  const queries = lookupQueries(text ?? "");
  if (queries.length === 0) {
    return undefined;
  }
  const data = await d.client().lookup(queries);
  const chosen = await pickAndCopy(
    d,
    lookupItems(data),
    `Results for ${queries.join(", ")}: choose one to copy it`,
  );
  return chosen?.copy;
}
