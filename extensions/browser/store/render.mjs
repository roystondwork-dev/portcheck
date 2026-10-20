// Renders the store listing images into store/out/ from the real extension:
// logo, promotional tiles, and 1280×800 screenshots around real popups.
// Needs `make build` (bin/portcheck) and `npm run build` (dist/).
//   node store/render.mjs
import { spawn } from "node:child_process";
import * as fs from "node:fs";
import * as os from "node:os";
import * as path from "node:path";
import { chromium } from "@playwright/test";

const ROOT = path.resolve(import.meta.dirname, "..");
const OUT = path.join(ROOT, "store", "out");
const DIST = path.join(ROOT, "dist");
const HOST = "io.github.roystondwork_dev.portcheck";
const EXT = "fmocabgaaihiflefcdafnhoihcibgnhl";
const ICON = fs.readFileSync(path.join(ROOT, "store", "icon.svg"), "utf8");
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
fs.rmSync(OUT, { recursive: true, force: true });
fs.mkdirSync(OUT, { recursive: true });

// portcheck as the native host, from a temporary copy (see e2e/extension.spec.ts).
const bin = path.join(fs.mkdtempSync(path.join(os.tmpdir(), "pc-store-")), "portcheck");
fs.copyFileSync(path.join(ROOT, "..", "..", "bin", "portcheck"), bin);
fs.chmodSync(bin, 0o755);

// A dev server started like a real one: zsh → npm → node.
const app = fs.mkdtempSync(path.join(os.tmpdir(), "shop-app-"));
fs.writeFileSync(path.join(app, "package.json"), JSON.stringify({ name: "shop-app", scripts: { dev: "node server.js" } }));
fs.writeFileSync(path.join(app, "server.js"), "require('http').createServer((q,r)=>r.end('shop')).listen(3000,'127.0.0.1')");
const server = spawn("zsh", ["-c", "npm run dev"], { cwd: app, env: { ...process.env, NODE_OPTIONS: "" }, detached: true });
await sleep(1500);

const profile = fs.mkdtempSync(path.join(os.tmpdir(), "pc-store-profile-"));
fs.mkdirSync(path.join(profile, "NativeMessagingHosts"));
fs.writeFileSync(
  path.join(profile, "NativeMessagingHosts", `${HOST}.json`),
  JSON.stringify({ name: HOST, description: "portcheck", path: bin, type: "stdio", allowed_origins: [`chrome-extension://${EXT}/`] }),
);
const ctx = await chromium.launchPersistentContext(profile, {
  channel: "chromium",
  colorScheme: "light",
  deviceScaleFactor: 2,
  args: [`--disable-extensions-except=${DIST}`, `--load-extension=${DIST}`],
});
const worker = ctx.serviceWorkers()[0] ?? (await ctx.waitForEvent("serviceworker"));

async function popup(url) {
  const page = await ctx.newPage();
  await page.goto(url).catch(() => undefined);
  let id = -1;
  for (let i = 0; i < 40 && id < 0; i++) {
    id = await worker.evaluate(async (u) => (await chrome.tabs.query({ url: u }))[0]?.id ?? -1, url);
    if (id < 0) await sleep(100);
  }
  const p = await ctx.newPage();
  await p.setViewportSize({ width: 380, height: 640 });
  await p.goto(`chrome-extension://${EXT}/popup.html?tab=${id}`);
  return p;
}

const shots = {};
{
  const p = await popup("http://localhost:3000/");
  await p.locator(".chip.running").waitFor();
  await p.locator(".port-row").first().waitFor();
  await sleep(300);
  shots.running = (await p.locator("body").screenshot()).toString("base64");
  await p.getByRole("button", { name: "Stop process" }).click();
  await sleep(200);
  shots.confirm = (await p.locator("body").screenshot()).toString("base64");
}
{
  const p = await popup("http://localhost:3999/");
  await p.locator(".chip.stopped").waitFor();
  await sleep(300);
  shots.stopped = (await p.locator("body").screenshot()).toString("base64");
}
await ctx.close();
process.kill(-server.pid);

// Compose the listing images.
const browser = await chromium.launch();
// Single quotes: this goes into a style="…" attribute.
const FONT = "font-family: 'Helvetica Neue', Helvetica, Arial, sans-serif;";
const BG = "background: radial-gradient(circle at 15% 20%, #5c6bc0 0, #283593 55%, #1a237e 100%);";

async function render(name, width, height, html, transparent = false) {
  const page = await browser.newPage({ viewport: { width, height }, deviceScaleFactor: 1 });
  await page.setContent(`<!doctype html><html><body style="margin:0;${FONT}">${html}</body></html>`);
  await page.screenshot({ path: path.join(OUT, name), omitBackground: transparent });
  await page.close();
}

const iconImg = (size) => `<div style="width:${size}px;height:${size}px">${ICON.replace("<svg ", `<svg width="${size}" height="${size}" `)}</div>`;

await render("logo-300.png", 300, 300, iconImg(300), true);

await render(
  "tile-small-440x280.png",
  440,
  280,
  `<div style="${BG}width:440px;height:280px;display:flex;align-items:center;gap:22px;padding:0 34px;box-sizing:border-box;color:#fff">
    ${iconImg(108)}
    <div><div style="font-size:34px;font-weight:750;letter-spacing:-0.02em">portcheck</div>
    <div style="font-size:17px;opacity:.9;margin-top:6px;line-height:1.35">Which process serves<br>your localhost page?</div></div></div>`,
);

await render(
  "tile-large-1400x560.png",
  1400,
  560,
  `<div style="${BG}width:1400px;height:560px;display:flex;align-items:center;gap:60px;padding:0 110px;box-sizing:border-box;color:#fff">
    ${iconImg(220)}
    <div><div style="font-size:72px;font-weight:750;letter-spacing:-0.03em">portcheck</div>
    <div style="font-size:32px;opacity:.92;margin-top:14px;line-height:1.35">See which process serves your localhost page,<br>who started it, and stop it safely.</div></div></div>`,
);

const screens = [
  {
    file: "screenshot-1-running.png",
    shot: shots.running,
    title: "Which process serves localhost:3000?",
    lines: ["The badge names it on every local tab.", "See who started it, its command, user and uptime.", "Every port on your machine, one click away."],
  },
  {
    file: "screenshot-2-stop.png",
    shot: shots.confirm,
    title: "Stop it safely",
    lines: ["Asks first, then stops the process.", "Never stops system processes or the browser.", "Containers get their own stop command."],
  },
  {
    file: "screenshot-3-not-running.png",
    shot: shots.stopped,
    title: "“This site can’t be reached”?",
    lines: ["The badge says off: nothing is listening.", "Your server isn't running: no network hunt.", "Works with any dev server: Node, Python, Go, Java…"],
  },
];
for (const s of screens) {
  await render(
    s.file,
    1280,
    800,
    `<div style="${BG}width:1280px;height:800px;display:flex;align-items:center;gap:70px;padding:0 90px;box-sizing:border-box;color:#fff">
      <div style="flex:1">
        <div style="display:flex;align-items:center;gap:14px;font-size:24px;font-weight:700;opacity:.95">${iconImg(44)} portcheck</div>
        <div style="font-size:50px;font-weight:750;letter-spacing:-0.02em;line-height:1.12;margin-top:28px">${s.title}</div>
        <ul style="margin:30px 0 0;padding:0;list-style:none;font-size:23px;line-height:1.5;opacity:.93">
          ${s.lines.map((l) => `<li style="margin:10px 0;display:flex;gap:12px"><span style="color:#a5d6a7">✓</span><span>${l}</span></li>`).join("")}
        </ul>
      </div>
      <img src="data:image/png;base64,${s.shot}" style="width:400px;border-radius:14px;box-shadow:0 24px 60px rgb(0 0 0 / 40%);background:#fff">
    </div>`,
  );
}
await browser.close();
console.log(`wrote ${fs.readdirSync(OUT).length} images to ${OUT}`);
