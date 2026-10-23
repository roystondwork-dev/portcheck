// The toolbar popup: the current local page's port, and the machine's ports.
import { ext } from "./api";
import { call, HostError, KNOWN_IDS, type SendNative } from "./host";
import { type LocalTarget, localTarget } from "./local";
import { listedPorts, portState } from "./model";
import type { CheckData, KillData, ListData } from "./schema";
import { type Action, filterPorts, type Icon, type PortView, portView, setupHelp } from "./view";

const send: SendNative = (host, message) => ext.runtime.sendNativeMessage(host, message);

type IconName = Icon | "refresh" | "help" | "stop" | "copy" | "check" | "external" | "search";

// Stroke icons, 24×24.
const ICONS: Record<IconName, string> = {
  tree: '<circle cx="6" cy="6" r="2.5"/><circle cx="6" cy="18" r="2.5"/><circle cx="18" cy="8" r="2.5"/><path d="M6 8.5v7M18 10.5c0 4-4 5-9.5 6"/>',
  terminal: '<path d="m5 17 5-5-5-5M12 19h7"/>',
  user: '<circle cx="12" cy="8" r="4"/><path d="M4.5 21a7.5 7.5 0 0 1 15 0"/>',
  clock: '<circle cx="12" cy="12" r="9"/><path d="M12 7v5l3 2"/>',
  box: '<path d="m21 8-9-5-9 5 9 5 9-5Z"/><path d="M3 8v8l9 5 9-5V8M12 13v8"/>',
  info: '<circle cx="12" cy="12" r="9"/><path d="M12 16v-4M12 8h.01"/>',
  plus: '<path d="M12 5v14M5 12h14"/>',
  refresh: '<path d="M20 12a8 8 0 1 1-2.4-5.7L20 8.5"/><path d="M20 3.5v5h-5"/>',
  help: '<circle cx="12" cy="12" r="9"/><path d="M9.5 9.2a2.6 2.6 0 0 1 5 .8c0 1.8-2.5 2.3-2.5 3.8M12 17h.01"/>',
  stop: '<rect x="7" y="7" width="10" height="10" rx="1.5"/>',
  copy: '<rect x="9" y="9" width="11" height="11" rx="2"/><path d="M5 15V6a2 2 0 0 1 2-2h9"/>',
  check: '<path d="m5 12.5 4.5 4.5L19 7.5"/>',
  external: '<path d="M7 17 17 7M9 7h8v8"/>',
  search: '<circle cx="11" cy="11" r="7"/><path d="m20 20-3.5-3.5"/>',
};

function icon(name: IconName): SVGSVGElement {
  // Parsed as SVG from the fixed markup above (no innerHTML).
  const svg = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" aria-hidden="true">${ICONS[name]}</svg>`;
  const doc = new DOMParser().parseFromString(svg, "image/svg+xml");
  return document.importNode(doc.documentElement, true) as unknown as SVGSVGElement;
}

function el<K extends keyof HTMLElementTagNameMap>(
  tag: K,
  props: Partial<HTMLElementTagNameMap[K]> = {},
  ...children: (Node | string)[]
): HTMLElementTagNameMap[K] {
  const e = Object.assign(document.createElement(tag), props);
  e.append(...children);
  return e;
}

function byId(id: string): HTMLElement {
  const e = document.getElementById(id);
  if (!e) {
    throw new Error(`missing #${id}`);
  }
  return e;
}

async function currentTab(): Promise<chrome.tabs.Tab | undefined> {
  // ?tab=<id> lets tests open the popup as a page for a given tab.
  const id = Number(new URLSearchParams(location.search).get("tab"));
  if (id > 0) {
    return ext.tabs.get(id);
  }
  const [tab] = await ext.tabs.query({ active: true, currentWindow: true });
  return tab;
}

function button(label: string, style: string, iconName?: IconName): HTMLButtonElement {
  const b = el("button", { className: `btn ${style}`, type: "button" });
  if (iconName) {
    b.append(icon(iconName));
  }
  b.append(label);
  return b;
}

function copyButton(text: string, label: string, style = "secondary"): HTMLButtonElement {
  const b = button(label, style, "copy");
  b.title = text;
  b.addEventListener("click", () => {
    void navigator.clipboard.writeText(text).then(() => {
      b.replaceChildren(icon("check"), "Copied");
      setTimeout(() => {
        b.replaceChildren(icon("copy"), label);
      }, 1500);
    });
  });
  return b;
}

function hero(address: string, chip?: { text: string; status: string }, summary?: string): HTMLElement {
  const status = el("div", { className: "status" });
  if (chip) {
    status.append(el("span", { className: `chip ${chip.status}`, textContent: chip.text }));
  }
  if (summary) {
    status.append(el("span", { className: chip ? "summary" : "muted", textContent: summary }));
  }
  return el("div", { className: "hero" }, el("div", { className: "address", textContent: address }), status);
}

function showSetup(section: HTMLElement, err: unknown): void {
  const e = err instanceof HostError ? err : new HostError(String(err), "failed");
  const help = setupHelp(e.problem, e.message, ext.runtime.id, KNOWN_IDS);
  const steps = el(
    "ol",
    { className: "steps" },
    ...help.steps.map((s) => {
      const li = el("li", {}, el("span", { textContent: s.text }));
      if (s.command !== undefined) {
        const copy = el("button", { className: "icon-btn", type: "button", title: "Copy" }, icon("copy"));
        copy.setAttribute("aria-label", `Copy ${s.command}`);
        copy.addEventListener("click", () => {
          void navigator.clipboard.writeText(s.command ?? "").then(() => {
            copy.replaceChildren(icon("check"));
          });
        });
        li.append(el("div", { className: "cmd" }, el("code", { className: "code", textContent: s.command }), copy));
      }
      return li;
    }),
  );
  section.replaceChildren(
    hero(help.headline, { text: "Not connected", status: "stopped" }),
    el("p", { className: "muted", textContent: help.intro }),
    steps,
  );
}

function renderView(section: HTMLElement, v: PortView, onStopped: (message: string, ok: boolean) => void): void {
  const parts: HTMLElement[] = [hero(v.address, { text: v.chip, status: v.status }, v.summary)];
  if (v.details.length > 0) {
    parts.push(
      el(
        "div",
        { className: "card" },
        ...v.details.map((d) =>
          el("div", { className: "detail" }, icon(d.icon), el(d.code ? "code" : "span", { textContent: d.text, title: d.text })),
        ),
      ),
    );
  }
  if (v.actions.length > 0) {
    const row = el("div", { className: "actions" });
    for (const a of v.actions) {
      row.append(a.kind === "copy" ? copyButton(a.text ?? "", a.label) : stopButton(a, v, row, onStopped));
    }
    parts.push(row);
  }
  section.replaceChildren(...parts);
}

/** "Stop process" asks for confirmation inline, then stops it with portcheck's guards. */
function stopButton(a: Action, v: PortView, row: HTMLElement, onStopped: (message: string, ok: boolean) => void): HTMLButtonElement {
  const b = button(a.label, a.style, "stop");
  b.addEventListener("click", () => {
    const name = v.summary.split(" · ")[0] ?? "the process";
    const yes = button("Stop", "danger");
    const force = button("Force kill", "secondary");
    const no = button("Cancel", "secondary");
    const panel = el(
      "div",
      { className: "confirm" },
      el("p", {}, "Stop ", el("strong", { textContent: name }), ` (pid ${a.pid ?? "?"})?`),
      el("div", { className: "actions" }, yes, force, no),
    );
    const stop = (forceKill: boolean) => async () => {
      for (const x of [yes, force, no]) {
        x.disabled = true;
      }
      yes.replaceChildren("Stopping…");
      try {
        const { data } = await call<KillData>(send, { op: "kill", pid: a.pid ?? 0, force: forceKill }, "kill");
        const r = data.results[0];
        if (r?.outcome === "failed") {
          onStopped(`Couldn't stop it: ${r.error ?? "unknown error"}`, false);
        } else {
          onStopped(`${r?.name ?? name} (pid ${a.pid ?? "?"}): ${r?.outcome ?? "stopped"}.`, true);
        }
      } catch (err) {
        onStopped(err instanceof Error ? err.message : String(err), false);
      }
    };
    yes.addEventListener("click", () => void stop(false)());
    force.addEventListener("click", () => void stop(true)());
    no.addEventListener("click", () => {
      panel.replaceWith(row);
    });
    row.replaceWith(panel);
  });
  return b;
}

let allPorts: { port: number; owners: string; url: string }[] = [];
let currentTarget: LocalTarget | undefined;

function renderPortList(): void {
  const list = byId("port-list");
  const rows = filterPorts(allPorts, (byId("filter") as HTMLInputElement).value, currentTarget?.port);
  byId("port-count").textContent = String(allPorts.length);
  if (rows.length === 0) {
    list.replaceChildren(el("li", { className: "empty", textContent: allPorts.length === 0 ? "Nothing is listening." : "No ports match." }));
    return;
  }
  list.replaceChildren(
    ...rows.map((p) => {
      const a = el("a", { className: "port-row", href: p.url, title: `Open ${p.url}` }, el("span", { className: "port", textContent: `:${p.port}` }), el("span", { className: "owners", textContent: p.owners }));
      if (p.current) {
        a.append(el("span", { className: "tag", textContent: "this tab" }));
      }
      a.append(icon("external"));
      a.addEventListener("click", (e) => {
        e.preventDefault();
        void ext.tabs.create({ url: p.url });
      });
      return el("li", {}, a);
    }),
  );
}

/** The localhost patterns in the manifest's host_permissions. */
const LOCAL_ORIGINS: string[] = (ext.runtime.getManifest() as chrome.runtime.ManifestV3).host_permissions ?? [];

/**
 * Firefox treats host permissions as optional, so the user may not have
 * allowed localhost access yet; without it the extension can't see tab
 * addresses. Returns true if access is granted (always, in Chrome and Edge).
 */
async function checkAccess(section: HTMLElement): Promise<boolean> {
  if (await ext.permissions.contains({ origins: LOCAL_ORIGINS })) {
    return true;
  }
  const allow = button("Allow access to localhost pages", "primary");
  allow.addEventListener("click", () => {
    void ext.permissions.request({ origins: LOCAL_ORIGINS }).then((granted) => {
      if (granted) {
        void refresh();
      }
    });
  });
  section.replaceChildren(
    hero("Allow localhost access", { text: "Permission needed", status: "denied" }),
    el("p", { className: "muted", textContent: "portcheck reads the address of localhost tabs only, to know which port a page uses. It can't see other sites." }),
    el("div", { className: "actions" }, allow),
  );
  return false;
}

async function renderTab(section: HTMLElement): Promise<void> {
  if (!(await checkAccess(section))) {
    return;
  }
  if (!currentTarget) {
    section.replaceChildren(hero("No local page", undefined, "Open a localhost page to see which process serves it."));
    return;
  }
  const target = currentTarget;
  section.replaceChildren(hero(target.label, undefined, "Checking…"));
  const { data } = await call<CheckData>(send, { op: "check", port: target.port }, "check");
  const view = portView(target.label, portState(data, target.port), Date.now());
  renderView(section, view, (message, ok) => {
    void refresh().then(() => {
      section.append(el("div", { className: `notice ${ok ? "good" : "bad"}`, textContent: message }));
    });
  });
}

async function renderPorts(): Promise<void> {
  const { data } = await call<ListData>(send, { op: "list" }, "list");
  allPorts = listedPorts(data);
  renderPortList();
}

async function refresh(): Promise<void> {
  const section = byId("tab");
  const r = byId("refresh");
  r.classList.add("spin");
  try {
    await Promise.all([renderTab(section), renderPorts()]);
    byId("ports").hidden = false;
  } catch (err) {
    showSetup(section, err);
    byId("ports").hidden = true;
  } finally {
    r.classList.remove("spin");
  }
}

byId("refresh").append(icon("refresh"));
byId("help").append(icon("help"));
byId("search-icon").replaceWith(icon("search"));
byId("refresh").addEventListener("click", () => void refresh());
byId("filter").addEventListener("input", renderPortList);

void currentTab().then((tab) => {
  currentTarget = localTarget(tab?.url);
  return refresh();
});
