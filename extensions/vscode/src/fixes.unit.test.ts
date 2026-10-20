import { describe, expect, it } from "vitest";
import { commandOnPort, type FixInput, planFixes, shellKind } from "./fixes";
import type { PortRow } from "./model";

const common = { id: "p", port: 3000, proto: "tcp", description: "", tooltip: "", contextValue: "port.process.tcp" } as const;
const proc: PortRow = { ...common, kind: "process", label: ":3000  node", pid: 4242, processName: "node", ancestry: "npm (4200) ← zsh (900)" };
const bare: PortRow = { ...common, kind: "process", label: ":3000  ruby", pid: 7 };
const container: PortRow = {
  ...proc, kind: "container", label: ":3000  docker:web", pid: 99,
  container: { name: "web", runtime: "docker", stopHint: "docker stop web" },
};
const hidden: PortRow = { ...common, kind: "hidden", label: ":3000  (owner hidden)" };

const base: FixInput = { port: 3000, status: "in-use", holders: [proc], shell: "posix", elevatedKill: "sudo portcheck kill 3000" };
const labels = (i: FixInput) => planFixes(i).map((f) => `${f.label} | ${f.description ?? ""}`);

describe("planFixes", () => {
  it("offers stop-and-re-run, re-run on a free port, and copy when the command is known", () => {
    expect(labels({ ...base, freePort: 3001, last: { commandLine: "npm run dev", finished: true } })).toEqual([
      "$(debug-restart) Stop node (pid 4242) and re-run | npm run dev",
      "$(debug-stop) Stop node (pid 4242) | started by npm (4200) ← zsh (900)",
      "$(arrow-right) Re-run on free port 3001 | PORT=3001 npm run dev",
      "$(copy) Copy free port 3001 | ",
      "$(info) Show details | ",
    ]);
    expect(planFixes({ ...base, last: { commandLine: "npm run dev", finished: true } })[0]?.action).toEqual({
      kind: "stop", row: proc, rerun: "npm run dev",
    });
  });

  it("doesn't offer re-runs while the command is still running or unknown", () => {
    const still = labels({ ...base, holders: [bare], freePort: 3001, last: { commandLine: "vite", finished: false } });
    expect(still).toEqual(["$(debug-stop) Stop ruby (pid 7) | ", "$(copy) Copy free port 3001 | ", "$(info) Show details | "]);
    expect(labels({ ...base, freePort: 3001 })).toHaveLength(3);
  });

  it("gives containers and hidden owners their commands, never a kill", () => {
    const fixes = planFixes({ ...base, holders: [container, hidden], last: { commandLine: "x", finished: true } });
    expect(fixes.map((f) => f.action)).toEqual([
      { kind: "runCommand", command: "docker stop web" },
      { kind: "runCommand", command: "sudo portcheck kill 3000" },
      { kind: "details" },
    ]);
    expect(fixes[0]?.label).toBe("$(package) Stop docker:web");
  });

  it("says when the port has been freed meanwhile", () => {
    expect(labels({ ...base, status: "free", holders: [] })).toEqual(["$(check) Port 3000 is free now | ", "$(info) Show details | "]);
    expect(planFixes({ ...base, status: "free", holders: [], last: { commandLine: "go run .", finished: true } })[0]).toEqual({
      label: "$(debug-restart) Port 3000 is free now: re-run", description: "go run .", action: { kind: "rerun", command: "go run ." },
    });
  });
});

describe("commandOnPort", () => {
  it.each([
    ["python3 -m http.server 3000", "python3 -m http.server 3001"],
    ["vite --port 3000 --host", "vite --port 3001 --host"],
    ["node server.js -p 3000", "node server.js -p 3001"],
    ["hugo server --bind 0.0.0.0:3000", "hugo server --bind 0.0.0.0:3001"],
    ["docker run -p 3000:80 nginx", "docker run -p 3001:80 nginx"],
    ["npx vite", "npx vite --port 3001"],
    ["ng serve", "ng serve --port 3001"],
    ["bin/rails server", "bin/rails server -p 3001"],
    ["python manage.py runserver", "python manage.py runserver 3001"],
    ["python3 -m http.server", "python3 -m http.server 3001"],
    ["npm run dev", "PORT=3001 npm run dev"],
    ["node app.js --retries 30000", "PORT=3001 node app.js --retries 30000"],
    ["curl 10.3000.1.1", "PORT=3001 curl 10.3000.1.1"],
  ])("%s", (cmd, want) => {
    expect(commandOnPort(cmd, 3000, 3001, "posix")).toBe(want);
  });

  it("sets PORT the way each shell does", () => {
    expect(commandOnPort("npm start", 3000, 3001, "powershell")).toBe("$env:PORT=3001; npm start");
    expect(commandOnPort("npm start", 3000, 3001, "cmd")).toBe("set PORT=3001&& npm start");
    expect(commandOnPort("npm start", 3000, 3001, "nushell")).toBe('with-env { PORT: "3001" } { npm start }');
  });
});

describe("shellKind", () => {
  it.each([
    ["/bin/zsh", "posix"],
    ["/opt/homebrew/bin/fish", "posix"],
    [undefined, "posix"],
    ["", "posix"],
    ["C:\\Program Files\\PowerShell\\7\\pwsh.exe", "powershell"],
    ["C:\\Windows\\System32\\WindowsPowerShell\\v1.0\\powershell.exe", "powershell"],
    ["C:\\Windows\\System32\\cmd.exe", "cmd"],
    ["/usr/local/bin/nu", "nushell"],
  ] as const)("%s → %s", (p, want) => {
    expect(shellKind(p)).toBe(want);
  });
});
