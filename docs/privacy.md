# Privacy

portcheck and its extensions for VS Code and the browser **collect no personal data**.
Nothing is sent to the portcheck project or to anyone else.

## The browser extension

- **What it reads:** only the address of tabs on your own machine (`localhost`,
  `*.localhost`, `127.0.0.1`, `[::1]`, `0.0.0.0`), to know which port a page uses. It
  can't see other websites, and it never reads page content.
- **Who it talks to:** only the `portcheck` program on your computer, through the
  browser's native messaging, and only after you set that up with
  `portcheck browser install`. portcheck answers four questions: its version, the ports
  listening on your machine, whether one port is in use and by which process, and a
  request to stop one process when you click **Stop**.
- **What it stores:** nothing. There are no accounts, analytics, ads or tracking, and no
  network requests to the internet.

Process information (names, IDs, command lines, users) is shown in the extension's
popup and tooltip only, and stays on your computer.

## The VS Code extension and the CLI

They run `portcheck` locally and show its results. They don't send anything over the
network, except:

- `portcheck scan`, which connects only to the hosts you name;
- `portcheck net --public`, which asks [ipify](https://www.ipify.org) (`api.ipify.org`
  and `api6.ipify.org`) for your public address, only when you pass `--public`;
- `portcheck net devices --probe`, which tries TCP connections to a few ports on the
  hosts of your local network, only when you pass `--probe`.
- uptime monitors you add with `portcheck monitor add` (or in VS Code's Network Tools),
  which send an HTTP GET request or open a TCP connection to each URL or address you
  added, on the schedule you chose, from your device. Results stay on your device.

`portcheck net` otherwise reads your own machine, and `net lookup` and device names
use your system's DNS resolver.

## Contact

Questions: open an issue at
<https://github.com/roystondwork-dev/portcheck/issues>.

*Last updated: 2026-10-20.*
