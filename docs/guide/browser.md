# Browser extension

The portcheck extension for **Chrome and Edge** (and Chromium, Brave) tells you which
process serves the localhost page you're looking at, and what's wrong when localhost
refuses to connect.

![The portcheck popup for localhost:3000: a green Running chip, node with its pid, a card with who started it, its command and user, Stop process and Copy PID buttons, and a filterable list of the ports listening on this machine](../assets/browser-popup.png){ width="380" }

[Get it from Microsoft Edge Add-ons](https://microsoftedge.microsoft.com/addons/detail/jenlahoamefdpfmpodhidkgcceikheee){ .md-button .md-button--primary }

## What it does

- **A badge on localhost tabs.** `node` (or `java`, `pyth`, `web` for a container…) shows
  what serves the page; hover for the process, its PID and who started it
  (`npm ← zsh`). **`off`** in red means nothing is listening: the server isn't running,
  so "This site can't be reached" isn't a network problem.
- **The popup** shows whether the page's port is **Running**, **Not running** or
  **Hidden**, and a card with who started the process, its command, its user and how
  long it has been running; for a Docker/Podman/WSL port, the command that stops it.
  **Stop process** asks first, then stops it with portcheck's
  [safety guards](kill.md#safety): it never stops system processes or the browser
  itself. **Copy PID** copies the process ID. Below, every port listening on the
  machine, filterable by port or process, one click to open; the current tab's port
  comes first. Light and dark themes follow your system.

It works on `localhost`, `*.localhost`, `127.0.0.1`, `[::1]` and `0.0.0.0`, on any port.

## Install

**Microsoft Edge:** install
[Portcheck - Localhost Port Inspector](https://microsoftedge.microsoft.com/addons/detail/jenlahoamefdpfmpodhidkgcceikheee) from Edge Add-ons, then do the
[setup](#setup) below.

**Chrome, Brave, Chromium:** the extension isn't in the Chrome Web Store yet. Install it
from a release:

1. Download `portcheck-browser-<version>.zip` from the
   [latest release](https://github.com/roystondwork-dev/portcheck/releases/latest) and
   unzip it into a folder you'll keep (the browser loads the extension from there).
2. Open `chrome://extensions` (or `brave://extensions`), turn on **Developer mode**,
   click **Load unpacked** and choose that folder.

## Setup

The extension asks the `portcheck` program on your machine, through the browser's
[native messaging](https://developer.chrome.com/docs/extensions/develop/concepts/native-messaging).
Connect them once:

1. [Install portcheck](../install.md) (0.8 or later; 0.7 also works, with the
   `--extension-id` command the popup shows).
2. Run:

    ```sh
    portcheck browser install
    ```

    This registers portcheck with every supported browser installed for your user;
    `--browser edge` picks one. No administrator rights are needed. Check with
    `portcheck browser status`; undo with `portcheck browser uninstall`.

If the popup says *Connect portcheck*, step 2 hasn't been done for this browser (or
portcheck moved: run it again). *Reconnect portcheck* means portcheck doesn't know this
copy of the extension yet: update portcheck, or run the exact command the popup shows.

## Troubleshooting

- **The popup says "Connect portcheck" or "Reconnect portcheck".** Run the command it
  shows (`portcheck browser install`, perhaps with `--extension-id`), then reopen the
  popup.
- **macOS asks "portcheck would like to access files in your Desktop folder"** (or
  Documents, Downloads). The registered portcheck binary lives in that folder, for
  example a build from source; the browser waits for your answer before the badge can
  appear. Click **Allow**, or register a copy outside those folders
  (`portcheck browser install --path /usr/local/bin/portcheck`). Package-manager
  installs (Homebrew, Scoop) aren't affected.
- **No badge on a localhost tab.** The badge only appears once the page has loaded;
  reload it. `portcheck browser status` shows whether this browser is registered.

## Firefox

The extension also runs in **Firefox 140 or later**. It's on its way to
[Firefox Add-ons](https://addons.mozilla.org/); until it's listed there, Firefox only
loads it as a temporary add-on: download `portcheck-firefox-<version>.zip` from the
[latest release](https://github.com/roystondwork-dev/portcheck/releases/latest), open
`about:debugging#/runtime/this-firefox`, click **Load Temporary Add-on…** and choose
the zip (Firefox removes temporary add-ons when it restarts).

Setup is the same: `portcheck browser install` registers portcheck with Firefox too
(`--browser firefox` to pick it). If the popup shows **Allow localhost access**, click
the button: Firefox lets you decide whether add-ons may read localhost addresses.

## Privacy and permissions

- The extension can only read the address of **localhost** tabs; it can't see other
  sites, and it has no access to page content.
- It can ask portcheck for exactly four things: its version, the listening ports,
  whether one port is in use, and to stop one process. portcheck refuses anything else.
  Nothing leaves your machine.

## Building from source

Requires Go 1.26+ and Node.js 22+.

```sh
git clone https://github.com/roystondwork-dev/portcheck.git && cd portcheck
make build                       # bin/portcheck (needs the browser commands)
bin/portcheck browser install --path "$PWD/bin/portcheck"
cd extensions/browser && npm ci && npm run build
```

Then in Chrome open `chrome://extensions` (Edge: `edge://extensions`), turn on
**Developer mode**, click **Load unpacked** and choose `extensions/browser/dist`.
