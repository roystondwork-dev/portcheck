# Menu bar / tray app

`portcheck-tray` keeps your ports one click away: in the macOS menu bar, or the system
tray on Windows and Linux. The icon shows how many ports your processes are listening
on; the menu lists them.

```text
 🔌 3
 ──────────────────────────────────────
 3 ports listening
 2 dev servers running over a day  ▸  Stop All 2
                                      :3000  node · api  (since Fri 6 Nov)
                                      :5173  node · shop  (since Sat 7 Nov)
 ──────────────────────────────────────
 :3000  node · api      ▸  Stop node (pid 41852)
 :5173  node · shop     ▸  Open http://localhost:3000/
 :8080  docker:web      ▸  Copy PID
                           Started by npm (41790) ← zsh (900)
                           Running since Fri 6 Nov 09:12
                           node ./node_modules/.bin/next dev
 ──────────────────────────────────────
 ☐ Show All Ports (also other users' and system)
 Refresh
 Quit portcheck
```

- Each port's submenu stops the process (portcheck's [safety guards](kill.md#safety)
  apply: never system processes), opens it in the browser, copies its PID or command
  line, and shows who started it and when. A port published by Docker, Podman or WSL
  offers **Copy Stop Command** instead.
- Each port shows the project it runs in (`node · shop`), found the same way as
  [`portcheck servers`](servers.md).
- **Forgotten dev servers:** when processes in your projects have been running for more
  than a day, a line at the top lists them, oldest first. Click one to stop it, or
  **Stop All**, with the same safety guards.
- By default it shows your own processes and containers, the ports you started.
  **Show All Ports** adds other users' processes and system services.
- The menu refreshes every 3 seconds; the result of the last action is in the icon's
  tooltip.

## Install

Download it from the [latest release](https://github.com/roystondwork-dev/portcheck/releases/latest):

=== "macOS"

    `portcheck-tray_<version>_darwin_universal.zip` holds **portcheck tray.app**
    (Apple Silicon and Intel, macOS 12 or later). Unzip it and move it to
    **Applications**. It isn't notarized by Apple, so the first time, **right-click →
    Open**, then **Open**. To start it at login: **System Settings → General → Login
    Items → +**.

=== "Windows"

    `portcheck-tray_<version>_windows_amd64.zip` (or `_arm64`) holds
    `portcheck-tray.exe`. To start it at login, put a shortcut to it in
    `shell:startup`.

=== "Linux"

    `portcheck-tray_<version>_linux_amd64.tar.gz` (or `_arm64`). It needs a desktop with
    a status area (StatusNotifierItem): KDE, Xfce and most others; on GNOME, the
    *AppIndicator and KStatusNotifierItem Support* extension.

The tray app reads ports the same way as `portcheck`; it doesn't need the CLI installed.
