# portcheck

See what's using a port and free it safely: which process, who started it, stop it.
Fixes "port already in use" (`EADDRINUSE`, `Address already in use`).

A safer [`kill-port`](https://roystondwork-dev.github.io/portcheck/fix/kill-port/):
it shows what it's about to stop and who started it, asks first, stops it gently
before forcing, and refuses to stop system processes.

```sh
npx portcheck-cli kill 3000    # free port 3000, after showing what's there
npx portcheck-cli 3000         # or: npm install -g portcheck-cli

portcheck 3000              # who holds port 3000? (exit 1 if in use)
portcheck kill 3000         # stop it, after confirming; never system processes
portcheck free              # a free port
portcheck                   # interactive view of every listening port
```

```text
PORT  PROTO  STATUS  PID    PROCESS  USER
3000  tcp    in-use  41852  node     alice
```

Works on macOS, Linux and Windows (x64 and arm64). On first run this package downloads the
native `portcheck` binary for your platform from the GitHub release, checks it against
checksums published with the package, and caches it.

## In your dev server

`npm install --save-dev portcheck-cli`, then let the dev server explain a taken port itself.

**Vite:** when your port is taken and Vite moves to the next one, the plugin says who
holds it; `k + enter` stops that process and restarts Vite on your port (Vite 7.3+).

```js
// vite.config.js
import portcheck from "portcheck-cli/vite";

export default { plugins: [portcheck()] };
```

**Node.js, Express, Next.js and others:** preload the hook. When a server can't listen
and the program exits with an error, it says who holds the port. Your program behaves
exactly as before: the error still reaches your handlers, or still crashes.

```sh
node -r portcheck-cli/register server.js
NODE_OPTIONS=--require=portcheck-cli/register npx next dev
```

Guide: https://roystondwork-dev.github.io/portcheck/guide/dev-servers/

## More

Also available for VS Code, JetBrains IDEs, Edge and Firefox, and through Homebrew,
Scoop and PyPI (`pipx install portcheck-tool`).

- Fix "port already in use" (Node.js, Python, Docker, macOS, Windows, Linux): https://roystondwork-dev.github.io/portcheck/fix/
- Documentation: https://roystondwork-dev.github.io/portcheck/
- Source (MIT): https://github.com/roystondwork-dev/portcheck
