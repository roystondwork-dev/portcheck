// End-to-end in a real Firefox (Selenium; Playwright can't load add-ons into
// Firefox). Registers the real portcheck with `portcheck browser install
// --browser firefox` (in the user's real Mozilla directory, removed afterwards),
// installs dist-firefox as a temporary add-on, and checks badge, popup and Stop.
// Needs: `make build` (bin/portcheck) and `npm run build:firefox`.
//   node --test e2e-firefox/
import assert from "node:assert/strict";
import { execFileSync, spawn } from "node:child_process";
import * as fs from "node:fs";
import * as os from "node:os";
import * as path from "node:path";
import { after, before, test } from "node:test";
import { Browser, Builder, By, until } from "selenium-webdriver";
import firefox from "selenium-webdriver/firefox.js";

const ROOT = path.resolve(import.meta.dirname, "..");
const BIN = process.env.PORTCHECK_BIN ?? path.join(ROOT, "..", "..", "bin", process.platform === "win32" ? "portcheck.exe" : "portcheck");
const ADDON_ID = "portcheck@roystondwork-dev.github.io";
const UUID = "6f1f1d1e-7b6a-4c39-9d36-2a7f1c0e5b11"; // fixed moz-extension:// host for the test
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

let host; // portcheck copy outside the repository (macOS prompts for ~/Desktop otherwise)
let xpi;

before(() => {
  assert.ok(fs.existsSync(BIN), `portcheck binary at ${BIN}; run "make build"`);
  const dist = path.join(ROOT, "dist-firefox");
  assert.ok(fs.existsSync(path.join(dist, "manifest.json")), 'run "npm run build:firefox"');
  const tmp = fs.mkdtempSync(path.join(os.tmpdir(), "pc-ff-"));
  host = path.join(tmp, path.basename(BIN));
  fs.copyFileSync(BIN, host);
  fs.chmodSync(host, 0o755);
  xpi = path.join(tmp, "portcheck.xpi");
  execFileSync("zip", ["-qr", xpi, "."], { cwd: dist });
  // The real registration command users run.
  const out = execFileSync(host, ["browser", "install", "--browser", "firefox", "--path", host], { encoding: "utf8", stdio: ["ignore", "pipe", "pipe"] });
  assert.match(out, /Mozilla Firefox/);
});

after(() => {
  if (host) execFileSync(host, ["browser", "uninstall", "--browser", "firefox"], { stdio: "ignore" });
});

async function launch(grantLocalhost) {
  const opts = new firefox.Options()
    .addArguments("-headless")
    .setPreference("extensions.webextensions.uuids", JSON.stringify({ [ADDON_ID]: UUID }))
    // Firefox treats Manifest V3 host permissions as optional; the add-on asks for
    // them in its popup. This pref grants them like an accepted install prompt.
    .setPreference("extensions.originControls.grantByDefault", grantLocalhost);
  // System access lets the test open the add-on's pages (see openAddonPage).
  const service = new firefox.ServiceBuilder().addArguments("--allow-system-access");
  const driver = await new Builder().forBrowser(Browser.FIREFOX).setFirefoxOptions(opts).setFirefoxService(service).build();
  await driver.installAddon(xpi, true);
  if (grantLocalhost) {
    // What clicking "Allow" in the popup does; temporary add-ons don't get the
    // install-time grant.
    await driver.setContext(firefox.Context.CHROME);
    const err = await driver.executeAsyncScript(
      `const done = arguments[arguments.length - 1];
       const { ExtensionPermissions } = ChromeUtils.importESModule("resource://gre/modules/ExtensionPermissions.sys.mjs");
       const ext = WebExtensionPolicy.getByID(arguments[0]).extension;
       ExtensionPermissions.add(arguments[0], { permissions: [], origins: ext.manifest.host_permissions }, ext).then(() => done(""), (e) => done(String(e)));`,
      ADDON_ID,
    );
    await driver.setContext(firefox.Context.CONTENT);
    assert.equal(err, "", "granting localhost access");
  }
  return driver;
}

function holdPort() {
  const env = { ...process.env };
  delete env.NODE_OPTIONS;
  const child = spawn(process.execPath, ["-e", "const s=require('http').createServer((q,r)=>r.end('hello')).listen(0,'127.0.0.1',()=>console.log('port='+s.address().port))"], { env });
  return new Promise((resolve, reject) => {
    let out = "";
    child.stdout.on("data", (d) => {
      out += d;
      const m = /port=(\d+)/.exec(out);
      if (m) resolve({ child, port: Number(m[1]) });
    });
    child.once("error", reject);
  });
}

/**
 * Opens one of the add-on's pages in a new tab and switches to it. WebDriver
 * may not navigate to moz-extension:// URLs itself, so the tab is opened from
 * Firefox's privileged (chrome) context.
 */
async function openAddonPage(driver, page) {
  const before = await driver.getAllWindowHandles();
  await driver.setContext(firefox.Context.CHROME);
  await driver.executeScript(
    `const tab = gBrowser.addTab(arguments[0], { triggeringPrincipal: Services.scriptSecurityManager.getSystemPrincipal() });
     gBrowser.selectedTab = tab;`,
    `moz-extension://${UUID}/${page}`,
  );
  await driver.setContext(firefox.Context.CONTENT);
  let handles = before;
  for (let i = 0; i < 40 && handles.length === before.length; i++) {
    await sleep(100);
    handles = await driver.getAllWindowHandles();
  }
  await driver.switchTo().window(handles.find((h) => !before.includes(h)));
}

/** Runs fn(arg) inside the add-on's page (it has the extension APIs). */
function inExtension(driver, fn, arg) {
  return driver.executeAsyncScript(
    `const done = arguments[arguments.length - 1];
     (${fn})(arguments[0]).then(done, (e) => done("ERROR " + e));`,
    arg,
  );
}

test("Firefox: badge, popup and Stop with the real portcheck", async () => {
  const { child, port } = await holdPort();
  const exited = new Promise((resolve) => child.once("exit", resolve));
  const driver = await launch(true);
  try {
    const url = `http://localhost:${port}/`;
    await driver.get(url);
    assert.equal(await driver.findElement(By.css("body")).getText(), "hello");

    await openAddonPage(driver, "popup.html");
    let tab = -1;
    let badge = "";
    for (let i = 0; i < 60 && badge !== "node"; i++) {
      [tab, badge] = await inExtension(driver, async (u) => {
        const [t] = await browser.tabs.query({ url: u });
        return t ? [t.id, await browser.action.getBadgeText({ tabId: t.id })] : [-1, ""];
      }, url);
      if (badge !== "node") await sleep(250);
    }
    if (badge !== "node") {
      // What the add-on sees, for the failure message.
      const seen = await inExtension(driver, async () => {
        const tabs = await browser.tabs.query({});
        const native = await browser.runtime.sendNativeMessage("io.github.roystondwork_dev.portcheck", { op: "version" }).then(
          (r) => JSON.stringify(r).slice(0, 120),
          (e) => "ERROR " + e.message,
        );
        return JSON.stringify({
          access: await browser.permissions.getAll(),
          tabs: await Promise.all(tabs.map(async (t) => [t.id, t.url, await browser.action.getBadgeText({ tabId: t.id })])),
          native,
        });
      });
      assert.fail(`badge on the localhost tab is "${badge}"; the add-on sees ${seen}`);
    }
    const title = await inExtension(driver, (t) => browser.action.getTitle({ tabId: t }), tab);
    assert.match(title, new RegExp(`localhost:${port} is served by node \\(pid ${child.pid}\\)`));

    await openAddonPage(driver, `popup.html?tab=${tab}`);
    const summary = await driver.wait(until.elementLocated(By.css(".summary")), 15_000);
    await driver.wait(until.elementTextIs(summary, `node · pid ${child.pid}`), 15_000);
    assert.equal(await driver.findElement(By.css(".chip")).getText().then((s) => s.toLowerCase()), "running");

    await driver.findElement(By.xpath("//button[contains(., 'Stop process')]")).click();
    await driver.findElement(By.xpath("//div[@class='confirm']//button[normalize-space(.)='Stop']")).click();
    await exited;
    const notice = await driver.wait(until.elementLocated(By.css(".notice")), 15_000);
    assert.equal(await notice.getText(), `node (pid ${child.pid}): terminated.`);
  } finally {
    await driver.quit();
    child.kill();
  }
});

test("Firefox: the popup asks for localhost access when it isn't granted", async () => {
  const driver = await launch(false);
  try {
    await openAddonPage(driver, "popup.html");
    const address = await driver.wait(until.elementLocated(By.css(".address")), 15_000);
    await driver.wait(until.elementTextIs(address, "Allow localhost access"), 15_000);
    assert.ok(await driver.findElement(By.xpath("//button[contains(., 'Allow access to localhost pages')]")).isDisplayed());
  } finally {
    await driver.quit();
  }
});
