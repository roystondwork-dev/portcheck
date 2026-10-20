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
}

/** Starts Chromium with the extension; registers portcheck in its profile unless told not to. */
async function launch(register: boolean): Promise<Browser> {
  const profile = fs.mkdtempSync(path.join(os.tmpdir(), "pc-chromium-"));
  if (register) {
    // Chromium also reads native messaging hosts from <profile>/NativeMessagingHosts.
    const dir = path.join(profile, "NativeMessagingHosts");
    fs.mkdirSync(dir);
    fs.writeFileSync(
      path.join(dir, `${HOST}.json`),
      JSON.stringify({ name: HOST, description: "portcheck (test)", path: BIN, type: "stdio", allowed_origins: [`chrome-extension://${EXT_ID}/`] }),
    );
  }
  const context = await chromium.launchPersistentContext(profile, {
    channel: "chromium",
    args: [`--disable-extensions-except=${DIST}`, `--load-extension=${DIST}`],
  });
  const worker = context.serviceWorkers()[0] ?? (await context.waitForEvent("serviceworker"));
  expect(worker.url()).toBe(`chrome-extension://${EXT_ID}/background.js`);
  return { context, worker };
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
      await expect(popup.locator("h1")).toHaveText(`127.0.0.1:${port} is served by node (pid ${child.pid})`);
      await expect(popup.locator("dt", { hasText: "Started by" })).toBeVisible();
      await expect(popup.locator("#port-list")).toContainText(`:${port}`);

      await popup.getByRole("button", { name: "Stop node" }).click();
      await expect(popup.getByText(`Stop node (pid ${child.pid})?`)).toBeVisible();
      await popup.getByRole("button", { name: "Stop", exact: true }).click();
      await exited;
      await expect(popup.locator(".message").last()).toHaveText(`node (pid ${child.pid}): terminated.`);
      await expect(popup.locator("h1")).toHaveText(`Nothing is listening on 127.0.0.1:${port}`);
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

test("without portcheck, the popup explains how to set it up", async () => {
  const b = await launch(false);
  try {
    const popup = await b.context.newPage();
    await popup.goto(`chrome-extension://${EXT_ID}/popup.html`);
    await expect(popup.locator("h1")).toHaveText("Connect portcheck to this browser");
    await expect(popup.locator("code", { hasText: "portcheck browser install" })).toBeVisible();
  } finally {
    await b.context.close();
  }
});
