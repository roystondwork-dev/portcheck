// What the popup shows, as data. Pure; unit-tested. popup.ts turns it into DOM.
import type { Holder, PortState } from "./model";
import type { HostProblem } from "./host";

export interface Line {
  label: string;
  value: string;
  /** Render as code. */
  code?: boolean;
}

export interface Action {
  kind: "stop" | "copy";
  label: string;
  /** For "stop": the process; for "copy": the text. */
  pid?: number | undefined;
  text?: string;
}

export interface PortView {
  headline: string;
  tone: "good" | "bad" | "muted";
  lines: Line[];
  actions: Action[];
}

function holderLines(h: Holder): Line[] {
  const lines: Line[] = [];
  if (h.ancestry !== "") {
    lines.push({ label: "Started by", value: h.ancestry });
  }
  if (h.cmdline !== undefined) {
    lines.push({ label: "Command", value: h.cmdline, code: true });
  }
  if (h.user !== undefined) {
    lines.push({ label: "User", value: h.user });
  }
  if (h.container) {
    lines.push({ label: "Stop with", value: h.container.stopHint, code: true });
  }
  return lines;
}

/** Actions for a visible holder: a process (has a pid) or a container. */
function holderActions(h: Holder): Action[] {
  if (h.container) {
    // Never stop a container's proxy: that would affect every container.
    return [{ kind: "copy", label: "Copy stop command", text: h.container.stopHint }];
  }
  return [{ kind: "stop", label: `Stop ${h.name}`, pid: h.pid }];
}

/** The popup's view of the current tab's port. */
export function portView(label: string, s: PortState): PortView {
  if (s.status === "free") {
    return {
      headline: `Nothing is listening on ${label}`,
      tone: "bad",
      lines: [{ label: "Tip", value: "Start your server, or check the port in its configuration." }],
      actions: [],
    };
  }
  if (s.status === "denied") {
    return { headline: `${label} can't be used by your user`, tone: "muted", lines: [], actions: [] };
  }
  const [first, ...rest] = s.holders;
  if (first === undefined || first.hidden) {
    return {
      headline: `${label} is served by a hidden process`,
      tone: "muted",
      lines: [{ label: "Why", value: "It belongs to another user. Run portcheck with elevated privileges to see it." }],
      actions: [{ kind: "copy", label: "Copy command", text: `sudo portcheck kill ${s.port}` }],
    };
  }
  const who = first.pid === undefined || first.container ? first.name : `${first.name} (pid ${first.pid})`;
  const lines = holderLines(first);
  for (const h of rest) {
    lines.push({ label: "Also", value: h.pid === undefined ? h.name : `${h.name} (pid ${h.pid})` });
  }
  return { headline: `${label} is served by ${who}`, tone: "good", lines, actions: holderActions(first) };
}

export interface SetupHelp {
  headline: string;
  steps: { text: string; command?: string }[];
}

/** What to tell the user when portcheck can't be reached. */
export function setupHelp(problem: HostProblem, message: string): SetupHelp {
  if (problem === "failed") {
    return { headline: "portcheck reported a problem", steps: [{ text: message }] };
  }
  const register = { text: "Then connect it to this browser:", command: "portcheck browser install" };
  if (problem === "forbidden") {
    return {
      headline: "portcheck needs to be connected again",
      steps: [{ text: "Update portcheck, then run:", command: "portcheck browser install" }],
    };
  }
  return {
    headline: "Connect portcheck to this browser",
    steps: [
      { text: "Install portcheck (macOS, Linux):", command: "brew install roystondwork-dev/tap/portcheck" },
      {
        text: "or on Windows:",
        command: "scoop bucket add portcheck https://github.com/roystondwork-dev/scoop-bucket; scoop install portcheck",
      },
      register,
    ],
  };
}
