// The Network Tools webview. It only collects input and draws the tables the
// extension sends back (see src/netPanelModel.ts); every text is set with
// textContent, never as HTML.
// @ts-check
(function () {
  // eslint-disable-next-line no-undef
  const vscode = acquireVsCodeApi();
  const state = vscode.getState() || {};

  /** @param {string} section */
  function out(section) {
    return /** @type {HTMLElement} */ (
      document.querySelector(`#${section} .out`)
    );
  }

  /**
   * @param {HTMLElement} el
   * @param {boolean} busy
   */
  function setBusy(el, busy) {
    const section = /** @type {HTMLElement} */ (el.closest("section"));
    for (const b of section.querySelectorAll("button")) {
      b.disabled = busy;
    }
    if (busy) {
      const o = out(section.id);
      o.replaceChildren(text("p", "Working…", "muted"));
    }
  }

  /**
   * @param {string} tag
   * @param {string} value
   * @param {string} [cls]
   */
  function text(tag, value, cls) {
    const el = document.createElement(tag);
    el.textContent = value;
    if (cls) {
      el.className = cls;
    }
    return el;
  }

  /** @param {any} msg */
  function send(msg) {
    vscode.postMessage(msg);
  }

  // Forms: Check ports and Look up.
  for (const form of document.querySelectorAll("form")) {
    const f = /** @type {HTMLFormElement} */ (form);
    for (const input of f.querySelectorAll("input")) {
      const saved = state[input.name];
      if (typeof saved === "string") {
        input.value = saved;
      }
    }
    f.addEventListener("submit", (e) => {
      e.preventDefault();
      const data = Object.fromEntries(new FormData(f));
      Object.assign(state, data);
      vscode.setState(state);
      setBusy(f, true);
      send({ type: f.dataset.request, ...data });
    });
  }

  // Buttons: My addresses and Devices.
  for (const button of document.querySelectorAll("button[data-request]")) {
    const b = /** @type {HTMLButtonElement} */ (button);
    b.addEventListener("click", () => {
      if (b.dataset.request !== "devices" || b.dataset.probe !== "true") {
        setBusy(b, true); // a probe asks first; it may be cancelled
      }
      send({
        type: b.dataset.request,
        public: b.dataset.public === "true",
        probe: b.dataset.probe === "true",
      });
    });
  }

  window.addEventListener("message", (event) => {
    /** @type {{type: string, section: string, columns: string[], rows: {cells: string[], tone?: string, copy?: string, stopPort?: number}[], summary?: string, error?: string}} */
    const r = event.data;
    if (r.type !== "result") {
      return;
    }
    const o = out(r.section);
    setBusy(o, false);
    if (r.error) {
      o.replaceChildren(text("p", r.error, "error"));
      return;
    }
    const parts = [];
    if (r.summary) {
      parts.push(text("p", r.summary, "summary"));
    }
    if (r.rows.length === 0) {
      parts.push(text("p", "Nothing found.", "muted"));
      o.replaceChildren(...parts);
      return;
    }
    const table = document.createElement("table");
    const head = table.createTHead().insertRow();
    for (const c of [...r.columns, ""]) {
      head.append(text("th", c));
    }
    const body = table.createTBody();
    for (const row of r.rows) {
      const tr = body.insertRow();
      row.cells.forEach((cell, i) => {
        const td = tr.insertCell();
        // The Status column of Check ports gets a coloured pill.
        if (i === 1 && row.tone && r.section === "ports") {
          td.append(text("span", cell, `pill ${row.tone}`));
        } else {
          td.textContent = cell;
          if (i === 0 && row.tone === "info") {
            td.className = "strong";
          }
        }
      });
      const actions = tr.insertCell();
      actions.className = "row-actions";
      if (row.copy) {
        const copy = text("button", "Copy", "link");
        copy.title = `Copy ${row.copy}`;
        copy.addEventListener("click", () =>
          send({ type: "copy", text: row.copy }),
        );
        actions.append(copy);
      }
      if (row.stopPort) {
        const stop = text("button", "Stop", "link danger");
        stop.title = `Stop what's using port ${row.stopPort} (asks first)`;
        stop.addEventListener("click", () =>
          send({ type: "stop", port: row.stopPort }),
        );
        actions.append(stop);
      }
    }
    parts.push(table);
    o.replaceChildren(...parts);
  });
})();
