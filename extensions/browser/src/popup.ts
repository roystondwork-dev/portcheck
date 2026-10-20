// The toolbar popup: the current local page's port, and the machine's ports.
import { call, HostError, type SendNative } from "./host";
import { localTarget } from "./local";
import { listedPorts, portState } from "./model";
import type { CheckData, KillData, ListData } from "./schema";
import { type Action, portView, setupHelp } from "./view";

const send: SendNative = (host, message) => chrome.runtime.sendNativeMessage(host, message);

function el<K extends keyof HTMLElementTagNameMap>(tag: K, props: Partial<HTMLElementTagNameMap[K]> = {}, ...children: (Node | string)[]): HTMLElementTagNameMap[K] {
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
    return chrome.tabs.get(id);
  }
  const [tab] = await chrome.tabs.query({ active: true, currentWindow: true });
  return tab;
}

function showSetup(section: HTMLElement, err: unknown): void {
  const e = err instanceof HostError ? err : new HostError(String(err), "failed");
  const help = setupHelp(e.problem, e.message);
  const steps = help.steps.map((s) =>
    el("li", {}, s.text, ...(s.command === undefined ? [] : [el("br"), el("code", { textContent: s.command }), " ", copyButton(s.command)])),
  );
  section.replaceChildren(el("h1", { className: "bad", textContent: help.headline }), el("ol", {}, ...steps));
}

function copyButton(text: string, label = "Copy"): HTMLButtonElement {
  const b = el("button", { textContent: label, type: "button" });
  b.addEventListener("click", () => {
    void navigator.clipboard.writeText(text).then(() => {
      b.textContent = "Copied";
    });
  });
  return b;
}

async function renderTab(section: HTMLElement, tab: chrome.tabs.Tab | undefined): Promise<void> {
  const target = localTarget(tab?.url);
  if (!target) {
    section.replaceChildren(el("p", { className: "muted", textContent: "Open a localhost page to see which process serves it." }));
    return;
  }
  section.replaceChildren(el("p", { className: "muted", textContent: `Checking ${target.label}…` }));
  const { data } = await call<CheckData>(send, { op: "check", port: target.port }, "check");
  const view = portView(target.label, portState(data, target.port));
  const dl = el("dl");
  for (const line of view.lines) {
    dl.append(el("dt", { textContent: line.label }), el("dd", {}, line.code ? el("code", { textContent: line.value }) : line.value));
  }
  const row = el("div", { className: "row" });
  const message = el("div", { className: "message" });
  for (const a of view.actions) {
    row.append(a.kind === "copy" ? copyButton(a.text ?? "", a.label) : stopButton(a, row, message, () => renderAll(tab)));
  }
  section.replaceChildren(el("h1", { className: view.tone, textContent: view.headline }), dl, row, message);
}

/** A Stop button that asks for confirmation inline, then stops the process. */
function stopButton(a: Action, row: HTMLElement, message: HTMLElement, refresh: () => Promise<void>): HTMLButtonElement {
  const b = el("button", { className: "danger", textContent: a.label, type: "button" });
  b.addEventListener("click", () => {
    const stop = (force: boolean) => async () => {
      row.replaceChildren(el("span", { className: "muted", textContent: "Stopping…" }));
      try {
        const { data } = await call<KillData>(send, { op: "kill", pid: a.pid ?? 0, force }, "kill");
        const r = data.results[0];
        message.textContent = r?.outcome === "failed" ? `Couldn't stop it: ${r.error ?? "unknown error"}` : `${r?.name ?? "Process"} (pid ${a.pid ?? "?"}): ${r?.outcome ?? "done"}.`;
      } catch (err) {
        message.textContent = err instanceof Error ? err.message : String(err);
      }
      const keep = message.textContent;
      await refresh();
      byId("tab").append(el("div", { className: "message", textContent: keep }));
    };
    const yes = el("button", { className: "danger", textContent: "Stop", type: "button" });
    const force = el("button", { textContent: "Force kill", type: "button" });
    const no = el("button", { textContent: "Cancel", type: "button" });
    yes.addEventListener("click", () => void stop(false)());
    force.addEventListener("click", () => void stop(true)());
    no.addEventListener("click", () => {
      row.replaceChildren(b);
    });
    row.replaceChildren(el("span", { textContent: `${a.label} (pid ${a.pid ?? "?"})?` }), yes, force, no);
  });
  return b;
}

async function renderPorts(list: HTMLElement): Promise<void> {
  const { data } = await call<ListData>(send, { op: "list" }, "list");
  const ports = listedPorts(data);
  if (ports.length === 0) {
    list.replaceChildren(el("li", { className: "muted", textContent: "Nothing is listening." }));
    return;
  }
  list.replaceChildren(
    ...ports.map((p) => {
      const a = el("a", { href: p.url, textContent: `:${p.port}` });
      a.addEventListener("click", (e) => {
        e.preventDefault();
        void chrome.tabs.create({ url: p.url });
      });
      return el("li", {}, a, `  ${p.owners}`);
    }),
  );
}

async function renderAll(tab: chrome.tabs.Tab | undefined): Promise<void> {
  const section = byId("tab");
  const list = byId("port-list");
  try {
    await Promise.all([renderTab(section, tab), renderPorts(list)]);
  } catch (err) {
    showSetup(section, err);
    byId("ports").hidden = true;
  }
}

void currentTab().then(renderAll);
