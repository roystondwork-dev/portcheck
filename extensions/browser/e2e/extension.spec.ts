// End-to-end: the built extension in a real Chromium, talking to the real
// portcheck binary through native messaging.
// Needs: `make build` at the repository root (or PORTCHECK_BIN) and `npm run build`.
import { type ChildProcess, spawn } from "node:child_process";
import * as fs from "node:fs";
import * as net from "node:net";
import * as os from "node:os";
import * as path from "node:path";
import { type BrowserContext, chromium, expect, test, type Worker } from "@playwright/test";

const ROOT = path.resolve(__dirname, "..");
const DIST = path.join(ROOT, "dist");
const BIN =
  process.env.PORTCHECK_BIN ?? path.join(ROOT, "..", "..", "bin", process.platform === "win32" ? "portcheck.exe" : "portcheck");
const HOST = "io.github.roystondwork_dev.portcheck";
const EXT_ID = "fmocabgaaihiflefcdafnhoihcibgnhl";

interface Browser {
  context: BrowserContext;
  worker: Worker;
  profile: string;
}

/**
 * The binary the browser starts: a copy outside the repository, because macOS
 * asks for permission when a browser starts a program inside ~/Desktop,
 * ~/Documents or ~/Downloads, and the badge would wait for that prompt.
 */
const hostBinary = (() => {
  let copy: string | undefined;
  return () => {
    if (copy === undefined) {
      const dir = fs.mkdtempSync(path.join(os.tmpdir(), "pc-host-"));
      copy = path.join(dir, path.basename(BIN));
      fs.copyFileSync(BIN, copy);
      fs.chmodSync(copy, 0o755);
    }
    return copy;
  };
})();

/** Registers portcheck in a profile, like `portcheck browser install` does for real. */
function registerHost(profile: string, allow = [EXT_ID]): void {
  // Chromium also reads native messaging hosts from <profile>/NativeMessagingHosts.
  const dir = path.join(profile, "NativeMessagingHosts");
  fs.mkdirSync(dir, { recursive: true });
  fs.writeFileSync(
    path.join(dir, `${HOST}.json`),
    JSON.stringify({
      name: HOST,
      description: "portcheck (test)",
      path: hostBinary(),
      type: "stdio",
      allowed_origins: allow.map((id) => `chrome-extension://${id}/`),
    }),
  );
}

/**
 * Starts Chromium with the extension; registers portcheck in its profile for
 * the given extension IDs (none: not registered).
 */
async function launch(register: boolean, opts: { dist?: string; allow?: string[] } = {}): Promise<Browser> {
  const dist = opts.dist ?? DIST;
  const profile = fs.mkdtempSync(path.join(os.tmpdir(), "pc-chromium-"));
  if (register) {
    registerHost(profile, opts.allow);
  }
  const context = await chromium.launchPersistentContext(profile, {
    channel: "chromium",
    args: [`--disable-extensions-except=${dist}`, `--load-extension=${dist}`],
  });
  const worker = context.serviceWorkers()[0] ?? (await context.waitForEvent("serviceworker"));
  if (dist === DIST) {
    expect(worker.url()).toBe(`chrome-extension://${EXT_ID}/background.js`);
  }
  return { context, worker, profile };
}

/** A separate node process holding a TCP port. */
function holdPort(): Promise<{ child: ChildProcess; port: number }> {
  const env = { ...process.env };
  delete env.NODE_OPTIONS; // the test runner's loaders would print to stdout
  const child = spawn(
    process.execPath,
    ["-e", "const s=require('http').createServer((q,r)=>r.end('hello')).listen(0,'127.0.0.1',()=>console.log('port='+s.address().port))"],
    { env },
  );
  return new Promise((resolve, reject) => {
    let out = "";
    child.stdout.on("data", (d: Buffer) => {
      out += d.toString();
      const m = /port=(\d+)/.exec(out);
      if (m) {
        resolve({ child, port: Number(m[1]) });
      }
    });
    child.once("error", reject);
  });
}

function freePort(): Promise<number> {
  return new Promise((resolve) => {
    const s = net.createServer().listen(0, "127.0.0.1", () => {
      const { port } = s.address() as net.AddressInfo;
      s.close(() => {
        resolve(port);
      });
    });
  });
}

/** The tab showing url; waits until the navigation has committed (error pages too). */
async function tabId(worker: Worker, url: string): Promise<number> {
  const find = () =>
    worker.evaluate(async (u) => {
      const [tab] = await chrome.tabs.query({ url: u });
      return tab?.id ?? -1;
    }, url);
  await expect.poll(find).toBeGreaterThan(0);
  return find();
}

function badgeOf(worker: Worker, id: number): Promise<{ text: string; title: string }> {
  return worker.evaluate(async (tabId) => ({
    text: await chrome.action.getBadgeText({ tabId }),
    title: await chrome.action.getTitle({ tabId }),
  }), id);
}

test.describe("with portcheck registered", () => {
  let b: Browser;
  test.beforeAll(async () => {
    expect(fs.existsSync(BIN), `portcheck binary at ${BIN}; run "make build"`).toBe(true);
    b = await launch(true);
  });
  test.afterAll(async () => {
    await b.context.close();
  });

  test("the badge names the process serving a localhost page", async () => {
    const { child, port } = await holdPort();
    try {
      const page = await b.context.newPage();
      const url = `http://127.0.0.1:${port}/`;
      await page.goto(url);
      await expect(page.locator("body")).toHaveText("hello");
      const id = await tabId(b.worker, url);
      await expect.poll(async () => (await badgeOf(b.worker, id)).text).toBe("node");
      const { title } = await badgeOf(b.worker, id);
      expect(title).toContain(`127.0.0.1:${port} is served by node (pid ${child.pid})`);
      await page.close();
    } finally {
      child.kill();
    }
  });

  test("the badge says off when nothing listens", async () => {
    const port = await freePort();
    const page = await b.context.newPage();
    const url = `http://127.0.0.1:${port}/`;
    await page.goto(url).catch(() => undefined); // net::ERR_CONNECTION_REFUSED
    const id = await tabId(b.worker, url);
    await expect.poll(async () => (await badgeOf(b.worker, id)).text).toBe("off");
    expect((await badgeOf(b.worker, id)).title).toBe(`Nothing is listening on 127.0.0.1:${port}. Is your server running?`);
    await page.close();
  });

  test("the popup shows the process and stops it after confirmation", async () => {
    const { child, port } = await holdPort();
    const exited = new Promise<void>((resolve) => child.once("exit", () => {
      resolve();
    }));
    try {
      const page = await b.context.newPage();
      const url = `http://127.0.0.1:${port}/`;
      await page.goto(url);
      const id = await tabId(b.worker, url);

      const popup = await b.context.newPage();
      await popup.goto(`chrome-extension://${EXT_ID}/popup.html?tab=${id}`);
      await expect(popup.locator(".address")).toHaveText(`127.0.0.1:${port}`);
      await expect(popup.locator(".chip")).toHaveText("Running");
      await expect(popup.locator(".summary")).toHaveText(`node · pid ${child.pid}`);
      await expect(popup.locator(".detail", { hasText: "Started by" })).toBeVisible();
      await expect(popup.locator(".port-row", { hasText: `:${port}` })).toContainText("this tab");

      // Filtering the ports list.
      await popup.locator("#filter").fill(String(port));
      await expect(popup.locator(".port-row")).toHaveCount(1);
      await popup.locator("#filter").fill("");

      // Cancel, then stop for real.
      await popup.getByRole("button", { name: "Stop process" }).click();
      await expect(popup.locator(".confirm p")).toHaveText(`Stop node (pid ${child.pid})?`);
      await popup.getByRole("button", { name: "Cancel" }).click();
      await expect(popup.locator(".confirm")).toHaveCount(0);
      expect(child.exitCode).toBeNull();
      await popup.getByRole("button", { name: "Stop process" }).click();
      await popup.getByRole("button", { name: "Stop", exact: true }).click();
      await exited;
      await expect(popup.locator(".notice")).toHaveText(`node (pid ${child.pid}): terminated.`);
      await expect(popup.locator(".chip")).toHaveText("Not running");
      await popup.close();
      await page.close();
    } finally {
      child.kill(); // no-op once stopped
    }
  });
});

test("a localhost page opened while the browser starts gets its badge", async () => {
  // Navigation events can arrive before the extension's worker is ready; the
  // worker checks open tabs when it starts.
  const { child, port } = await holdPort();
  const b = await launch(true);
  try {
    const page = await b.context.newPage();
    const url = `http://localhost:${port}/`;
    await page.goto(url);
    const id = await tabId(b.worker, url);
    await expect.poll(async () => (await badgeOf(b.worker, id)).text).toBe("node");
    expect((await badgeOf(b.worker, id)).title).toContain(`localhost:${port} is served by node (pid ${child.pid})`);
  } finally {
    await b.context.close();
    child.kill();
  }
});

test("without portcheck, the popup gives the one-line installer, then connects by itself", async () => {
  const b = await launch(false);
  try {
    const popup = await b.context.newPage();
    await popup.goto(`chrome-extension://${EXT_ID}/popup.html`);
    await expect(popup.locator(".address")).toHaveText("Connect portcheck");
    await expect(popup.locator(".chip")).toHaveText("Not connected");
    const installer =
      process.platform === "win32"
        ? "irm https://roystondwork-dev.github.io/portcheck/install.ps1 | iex"
        : "curl -fsSL https://roystondwork-dev.github.io/portcheck/install.sh | sh";
    await expect(popup.locator("code")).toHaveText(installer);
    await expect(popup.locator(".waiting")).toBeVisible();
    await expect(popup.locator("#ports")).toBeHidden();

    // The user runs the installer; the open popup notices without a reload.
    registerHost(b.profile);
    await expect(popup.locator(".address")).toHaveText("No local page", { timeout: 15_000 });
    await expect(popup.locator("#ports")).toBeVisible();
  } finally {
    await b.context.close();
  }
});

test("without portcheck, the badge and popup still say whether the server runs", async () => {
  const { child, port } = await holdPort();
  const closed = await freePort();
  const b = await launch(false);
  try {
    const page = await b.context.newPage();
    const up = `http://localhost:${port}/`;
    await page.goto(up);
    const id = await tabId(b.worker, up);
    await expect.poll(async () => (await badgeOf(b.worker, id)).text).toBe("on");
    expect((await badgeOf(b.worker, id)).title).toBe(`localhost:${port} is running. Click to connect the portcheck app and see which process it is.`);

    const popup = await b.context.newPage();
    await popup.goto(`chrome-extension://${EXT_ID}/popup.html?tab=${id}`);
    await expect(popup.locator(".address")).toHaveText(`localhost:${port}`);
    await expect(popup.locator(".chip")).toHaveText("Running");
    await expect(popup.locator(".setup-title")).toHaveText("Connect portcheck");

    const down = `http://localhost:${closed}/`;
    await page.goto(down).catch(() => undefined); // connection refused
    const id2 = await tabId(b.worker, down);
    await expect.poll(async () => (await badgeOf(b.worker, id2)).text).toBe("off");
  } finally {
    await b.context.close();
    child.kill();
  }
});

test("a store build (another ID) asks to be allowed, then works", async () => {
  // Store builds have no manifest key, so the browser assigns another ID.
  const storeDist = path.join(ROOT, "dist-store");
  expect(fs.existsSync(storeDist), 'run "npm run build:store"').toBe(true);
  const first = await launch(true, { dist: storeDist });
  const id = new URL(first.worker.url()).host;
  expect(id).not.toBe(EXT_ID);
  try {
    const popup = await first.context.newPage();
    await popup.goto(`chrome-extension://${id}/popup.html`);
    await expect(popup.locator(".address")).toHaveText("Reconnect portcheck");
    await expect(popup.locator("code").nth(1)).toHaveText(`portcheck browser install --extension-id ${id}`);
  } finally {
    await first.context.close();
  }

  // After `portcheck browser install --extension-id <id>` it connects.
  const { child, port } = await holdPort();
  const second = await launch(true, { dist: storeDist, allow: [EXT_ID, id] });
  try {
    expect(new URL(second.worker.url()).host).toBe(id);
    const page = await second.context.newPage();
    const url = `http://127.0.0.1:${port}/`;
    await page.goto(url);
    const tab = await tabId(second.worker, url);
    await expect.poll(async () => (await badgeOf(second.worker, tab)).text).toBe("node");
  } finally {
    await second.context.close();
    child.kill();
  }
});
