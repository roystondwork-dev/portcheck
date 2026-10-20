// Turns `portcheck doctor` results for a .portcheck.yaml into what the editor
// shows: a status bar summary and problems on the file. Pure, unit-tested.
import type { DoctorData } from "./schema";

export interface Problem {
  /** 0-based line in .portcheck.yaml. */
  line: number;
  message: string;
  /** "error": the file is invalid; "warning": a port check failed. */
  severity: "error" | "warning";
}

/** Problems from doctor's error output for an invalid file ("… line 3: field prot not found"). */
export function configErrors(detail: string): Problem[] {
  const found: Problem[] = [];
  for (const m of detail.matchAll(/line (\d+): (.+)/g)) {
    found.push({ line: Math.max(Number(m[1]) - 1, 0), message: (m[2] as string).trim(), severity: "error" });
  }
  if (found.length > 0) {
    return found;
  }
  const message = detail.replace(/^.*?invalid \.portcheck\.ya?ml: /s, "").trim();
  return [{ line: 0, message: message || "invalid .portcheck.yaml", severity: "error" }];
}

/** One warning per failed check, on the line of its "name:". */
export function checkProblems(text: string, data: DoctorData): Problem[] {
  const lines = text.split(/\r?\n/);
  return data.checks
    .filter((c) => !c.ok)
    .map((c) => {
      const name = new RegExp(`^\\s*(?:-\\s*)?name:\\s*["']?${escapeRegExp(c.name)}["']?\\s*(?:#.*)?$`);
      const line = Math.max(
        lines.findIndex((l) => name.test(l)),
        0,
      );
      const fix = c.fix ? `. Fix: ${c.fix}` : "";
      return { line, message: `${c.name} (port ${c.port}): ${c.detail ?? c.status}${fix}`, severity: "warning" as const };
    });
}

export interface Summary {
  text: string;
  tooltip: string;
  ok: boolean;
}

/** The status bar item for a project. */
export function summarize(data: DoctorData): Summary {
  const failed = data.checks.filter((c) => !c.ok);
  const n = data.checks.length;
  const lines = data.checks.map(
    (c) => `${c.ok ? "$(pass)" : "$(warning)"} **${c.name}** ${c.port}/${c.proto}: ${c.ok ? "ok" : (c.detail ?? c.status)}`,
  );
  return {
    text: failed.length === 0 ? `$(pass) ports` : `$(warning) ${failed.length}/${n} ports`,
    tooltip: [`**Project ports** (${data.config})`, ...lines, "", "Click to fix or re-check."].join("\n\n"),
    ok: failed.length === 0,
  };
}

/** Port → name of the ports a project declares. */
export function projectPorts(data: DoctorData): Map<number, string> {
  return new Map(data.checks.map((c) => [c.port, c.name]));
}

function escapeRegExp(s: string): string {
  return s.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}
