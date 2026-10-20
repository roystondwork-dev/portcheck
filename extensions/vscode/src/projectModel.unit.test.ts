import { readFileSync } from "node:fs";
import * as path from "node:path";
import { describe, expect, it } from "vitest";
import { checkProblems, configErrors, projectPorts, summarize } from "./projectModel";
import type { DoctorData, Envelope } from "./schema";

const doctor = (JSON.parse(
  readFileSync(path.join(__dirname, "..", "..", "..", "internal", "output", "testdata", "doctor.json.golden"), "utf8"),
) as Envelope<DoctorData>).data;

const yaml = `# yaml-language-server: $schema=…
version: 1
ports:
  - name: db
    port: 5432
    expect: open
  - port: 3000
    name: "web" # the app
`;

describe("configErrors (real portcheck messages)", () => {
  it("reads line numbers", () => {
    expect(configErrors("/w/.portcheck.yaml: invalid .portcheck.yaml: yaml: unmarshal errors:\n  line 3: field prot not found\n  line 7: field x not found")).toEqual([
      { line: 2, message: "field prot not found", severity: "error" },
      { line: 6, message: "field x not found", severity: "error" },
    ]);
    expect(configErrors("/w/.portcheck.yaml: invalid .portcheck.yaml: yaml: line 0: oops")).toEqual([
      { line: 0, message: "oops", severity: "error" },
    ]);
  });

  it("puts errors without a line at the top", () => {
    expect(configErrors("/w/.portcheck.yaml: invalid .portcheck.yaml: version must be 1 (got 0)")).toEqual([
      { line: 0, message: "version must be 1 (got 0)", severity: "error" },
    ]);
    expect(configErrors("")).toEqual([{ line: 0, message: "invalid .portcheck.yaml", severity: "error" }]);
  });
});

describe("checkProblems", () => {
  it("puts a warning on the failed entry's name line", () => {
    expect(checkProblems(yaml, doctor)).toEqual([
      {
        line: 7,
        message: "web (port 3000): in use by node (pid 4242), started by npm (4200). Fix: portcheck kill 3000",
        severity: "warning",
      },
    ]);
  });

  it("falls back to the first line and to the status", () => {
    const data: DoctorData = { config: "c", ok: false, checks: [{ name: "a.b", port: 1, proto: "tcp", expect: "open", ok: false, status: "closed" }] };
    expect(checkProblems("name: axb\r\n", data)).toEqual([{ line: 0, message: "a.b (port 1): closed", severity: "warning" }]);
  });
});

describe("summarize", () => {
  it("counts problems", () => {
    const s = summarize(doctor);
    expect(s).toMatchObject({ text: "$(warning) 1/2 ports", ok: false });
    expect(s.tooltip).toContain("$(warning) **web** 3000/tcp: in use by node (pid 4242)");
    expect(s.tooltip).toContain("$(pass) **db** 5432/tcp: ok");
    expect(summarize({ ...doctor, checks: doctor.checks.filter((c) => c.ok) })).toMatchObject({ text: "$(pass) ports", ok: true });
    const noDetail = summarize({ ...doctor, checks: [{ name: "x", port: 1, proto: "tcp", expect: "free", ok: false, status: "in-use" }] });
    expect(noDetail.tooltip).toContain("**x** 1/tcp: in-use");
  });

  it("maps ports to names", () => {
    expect(projectPorts(doctor)).toEqual(new Map([[3000, "web"], [5432, "db"]]));
  });
});
