// Sets the toolbar badge for local pages: which process serves the page, or
// "off" when nothing listens (e.g. on "This site can't be reached").
import { ext } from "./api";
import { call, HostError, type SendNative } from "./host";
import { localTarget } from "./local";
import { badge, portState } from "./model";
import type { CheckData } from "./schema";

const send: SendNative = (host, message) => ext.runtime.sendNativeMessage(host, message);

async function setBadge(tabId: number, text: string, color: string, title: string): Promise<void> {
  await Promise.all([
    ext.action.setBadgeText({ tabId, text }),
    ext.action.setBadgeBackgroundColor({ tabId, color }),
    ext.action.setTitle({ tabId, title }),
  ]);
}

/** The latest update started per tab, so older answers can't overwrite newer ones. */
const latest = new Map<number, number>();

/** Updates the badge of one tab. */
async function update(tabId: number, url: string | undefined): Promise<void> {
  const generation = (latest.get(tabId) ?? 0) + 1;
  latest.set(tabId, generation);
  const current = () => latest.get(tabId) === generation;

  const target = localTarget(url);
  if (!target) {
    await setBadge(tabId, "", "#616161", "portcheck");
    return;
  }
  let b: { text: string; color: string; title: string };
  try {
    const { data } = await call<CheckData>(send, { op: "check", port: target.port }, "check");
    b = badge(target.label, portState(data, target.port));
  } catch (err) {
    const msg = err instanceof HostError ? err.message : String(err);
    b = { text: "set", color: "#616161", title: `${msg} Click for setup.` };
  }
  if (current()) {
    await setBadge(tabId, b.text, b.color, b.title);
  }
}

/** Updates every open tab: events from before the worker started are lost. */
async function updateAll(): Promise<void> {
  for (const tab of await ext.tabs.query({})) {
    if (tab.id !== undefined) {
      void update(tab.id, tab.url).catch(() => undefined);
    }
  }
}

ext.tabs.onUpdated.addListener((tabId, info, tab) => {
  // Chrome resets a tab's badge when a navigation commits, so update once the
  // page has loaded. "complete" also fires for error pages such as
  // ERR_CONNECTION_REFUSED.
  if (info.status === "complete") {
    void update(tabId, tab.url).catch(() => undefined); // the tab may be gone
  }
});

ext.tabs.onActivated.addListener(({ tabId }) => {
  void ext.tabs.get(tabId).then((tab) => update(tabId, tab.url)).catch(() => undefined);
});

ext.tabs.onRemoved.addListener((tabId) => {
  latest.delete(tabId);
});

void updateAll();
