# Firefox Add-ons (AMO) listing

Submit at https://addons.mozilla.org/developers/addon/submit/ (free; sign in with a
Mozilla account). portcheck must already support Firefox in its latest release (v0.9.0
or later), because the reviewers test with it.

## Upload

1. **How to distribute:** "On this site" (listed).
2. **Upload:** `portcheck-firefox-X.Y.Z.zip` from the GitHub release.
3. **Compatibility:** tick **Firefox** only, not Firefox for Android (native messaging
   doesn't exist on Android). The linter's one warning is about Android and can be
   ignored.
4. **Source code:** "Yes" → upload `portcheck-firefox-source-X.Y.Z.zip` from the same
   release (the add-on is bundled with esbuild; `BUILD-FIREFOX.md` inside explains the
   build).

## Describe the add-on

- **Name** and **summary:** from the manifest.
- **Add-on URL:** `portcheck`
- **Categories:** Web Development (and Other, if a second is allowed)
- **Support website:** https://github.com/roystondwork-dev/portcheck/issues
- **Homepage:** https://roystondwork-dev.github.io/portcheck/guide/browser/
- **License:** MIT
- **Privacy policy:** yes, paste: "portcheck collects no data. The add-on reads the
  address of localhost tabs only and talks only to the portcheck app on your computer
  through native messaging; nothing leaves your computer. Full policy:
  https://roystondwork-dev.github.io/portcheck/privacy/"
- **Description:**

```
Your dev server won't start because the port is taken, or localhost says "Unable to connect"? portcheck tells you what's going on, right in the toolbar.

• A badge on every localhost tab names the process serving it (node, python, java…); hover for its process ID and who started it (npm ← zsh).
• The badge says "off" when nothing is listening, so you know your server isn't running instead of hunting for network problems.
• The popup shows the process, its command, its user and how long it has been running, and stops it safely after you confirm. It never stops system processes or the browser itself. Ports published by Docker, Podman or WSL get the right stop command instead.
• Every port listening on your computer, filterable, one click to open.

Works on localhost, *.localhost, 127.0.0.1, [::1] and 0.0.0.0, on any port.

Requires the free, open-source portcheck app (macOS, Linux, Windows). Install it, then run "portcheck browser install" once; the add-on shows the steps. Nothing leaves your computer: no accounts, analytics or tracking.

Source and documentation: https://github.com/roystondwork-dev/portcheck
```

- **Screenshots:** `store/out/screenshot-1-running.png`, `screenshot-2-stop.png`,
  `screenshot-3-not-running.png` (run `node store/render.mjs` to regenerate).

## Notes to reviewer

```
The add-on works together with the free, open-source portcheck app (native messaging host "io.github.roystondwork_dev.portcheck"). No account or login is needed; nothing is sent over the network.

To test on Ubuntu:
1. Download portcheck_X.Y.Z_linux_amd64.tar.gz (or _arm64) from https://github.com/roystondwork-dev/portcheck/releases/latest and extract it.
2. Run: ./portcheck browser install --browser firefox
   (registers the app in ~/.mozilla/native-messaging-hosts for this add-on's ID, portcheck@roystondwork-dev.github.io)
3. Start a local server: python3 -m http.server 8000 and open http://localhost:8000.
4. The toolbar badge shows the process ("pyth"); the popup shows its details. If the popup shows "Allow localhost access", click "Allow access to localhost pages" (Firefox makes host permissions optional).
5. In the popup, click "Stop process", then "Stop": the server stops; reloading the page shows the "off" badge.

The JavaScript is bundled (not minified) from TypeScript with esbuild; the source archive and BUILD-FIREFOX.md rebuild it exactly (npm ci && npm run build:firefox).
```
