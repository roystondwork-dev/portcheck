# Edge Add-ons listing

Everything Partner Center asks for, in order
([Microsoft's guide](https://learn.microsoft.com/microsoft-edge/extensions/publish/publish-extension)).
Images: run `node store/render.mjs` (after `make build` and `npm run build`); they land
in `store/out/`.

## Package

Upload `portcheck-browser-store-<version>.zip` from the GitHub release (built with
`npm run build:store`: the release zip without the manifest `key`, since the store
assigns the ID).

## Availability

- **Visibility:** Public
- **Markets:** all

## Properties

- **Category:** Developer tools
- **Website:** https://roystondwork-dev.github.io/portcheck/guide/browser/
- **Support contact:** https://github.com/roystondwork-dev/portcheck/issues
- **Mature content:** no

## Privacy

**Single purpose:**

> Shows which local process serves the localhost page in the current tab (its name,
> process ID and who started it) and lets the user stop that process, using the
> portcheck app installed on the user's computer.

**Permission justification:**

- `nativeMessaging`: The extension gets port and process information from the portcheck
  app on the user's own computer, which the user connects with
  `portcheck browser install`. It asks for the listening ports, whether one port is in
  use and by which process, and, when the user clicks Stop and confirms, to stop that
  process.
- Host permissions (`localhost`, `*.localhost`, `127.0.0.1`, `0.0.0.0`, `[::1]`): Needed
  to read the address of tabs on the user's own machine, to know which port a page uses.
  The extension can't see any other site and never reads page content.

**Remote code:** No, I am not using remote code.

**Data usage:** collects none of the listed data types. Certify all three statements
(no selling or transferring data, no unrelated use, no creditworthiness use).

**Privacy policy URL:** https://roystondwork-dev.github.io/portcheck/privacy/

## Store listing (English)

- **Extension name** and **short description**: from the manifest.
- **Description:**

> Your dev server won't start because the port is taken, or localhost says "This site
> can't be reached"? portcheck tells you what's going on, right in the toolbar.
>
> • A badge on every localhost tab names the process serving it (node, python, java…);
> hover for its process ID and who started it (npm ← zsh).
> • The badge says "off" when nothing is listening, so you know your server isn't
> running instead of hunting for network problems.
> • The popup shows the process, its command, its user and how long it has been running,
> and stops it safely after you confirm. It never stops system processes or the browser
> itself. Ports published by Docker, Podman or WSL get the right stop command instead.
> • Every port listening on your computer, filterable, one click to open.
>
> Works on localhost, *.localhost, 127.0.0.1, [::1] and 0.0.0.0, on any port.
>
> Requires the free, open-source portcheck app (macOS, Linux, Windows). Install it, then
> run "portcheck browser install" once; the extension shows the steps. Nothing leaves
> your computer: no accounts, analytics or tracking.
>
> Source and documentation: https://github.com/roystondwork-dev/portcheck

- **Extension logo:** `logo-300.png`
- **Small promotional tile:** `tile-small-440x280.png`
- **Large promotional tile:** `tile-large-1400x560.png`
- **Screenshots:** `screenshot-1-running.png`, `screenshot-2-stop.png`,
  `screenshot-3-not-running.png`
- **Search terms** (max 7, 30 characters each, 21 words): `localhost`,
  `port in use`, `EADDRINUSE`, `dev server`, `kill process`, `port checker`,
  `developer tools`

## Notes for certification

> This extension works together with the free, open-source portcheck app on the user's
> computer (native messaging host "io.github.roystondwork_dev.portcheck").
>
> To test on Windows:
> 1. Download portcheck_<version>_windows_amd64.zip from
>    https://github.com/roystondwork-dev/portcheck/releases/latest, unzip it, and open a
>    terminal in that folder.
> 2. Run: .\portcheck.exe browser install --browser edge
> 3. Open the extension's popup. Because the store assigns a new extension ID, it shows
>    "Reconnect portcheck" with the exact command to run, for example:
>    .\portcheck.exe browser install --browser edge --extension-id <this extension's ID>
>    Run it, then reopen the popup.
> 4. Start any local server, e.g. "python -m http.server 8000", and open
>    http://localhost:8000. The toolbar badge shows the process ("pyth"); the popup shows
>    its details. Click "Stop process", then "Stop" to stop it; the badge turns to "off"
>    after a reload.
>
> No account or login is needed. Nothing is sent over the network.
