---
title: "Dev servers: Vite plugin and Node.js hook"
description: "Make Vite, Next.js, Express and other Node.js dev servers say who holds their port when it's taken, and stop it with one key: the portcheck-cli Vite plugin and Node.js preload hook."
---

# Dev servers: Vite plugin and Node.js hook

The `portcheck-cli` npm package plugs into your dev server, so a taken port is explained
where you see it: in the dev server's own output. Install it in your project:

```sh
npm install --save-dev portcheck-cli
```

## Vite

When the port you asked for is taken, Vite quietly moves to the next one
(`Port 5173 is in use, trying another one...`), and your app runs on 5174. The plugin
says who holds 5173, and adds a shortcut to take it back:

```ts title="vite.config.ts"
import { defineConfig } from "vite";
import portcheck from "portcheck-cli/vite";

export default defineConfig({
  plugins: [portcheck()],
});
```

```console
$ npx vite
Port 5173 is in use, trying another one...

  VITE v8.3.5  ready in 91 ms

  ➜  Local:   http://localhost:5174/
  ➜  press h + enter to show help
  portcheck: port 5173 is in use by node (pid 41852), started by npm (41850) ← zsh (900).
  portcheck: Stop it: npx portcheck-cli kill 5173
  portcheck: press k + enter to stop node and restart on port 5173
k
  portcheck: stopped node (pid 41852), restarting on port 5173
```

++k++ then ++enter++ checks that the same process still holds the port, stops it with
[`kill`'s guards](kill.md#safety) (never system processes), and restarts Vite on the
port you asked for. Ports published by a container get the right `docker stop` command
instead, and there's no shortcut when several processes hold the port.

The plugin only runs with `vite` (dev), not `vite build`. The shortcut needs Vite 7.3 or
later, and a terminal: in CI, or on older Vite, you get the explanation and the
`kill` command to run yourself.

## Node.js, Next.js, Express and others

For any Node.js program, preload the hook. When a server can't listen because its port is
taken and the program then exits with an error, it says who holds the port:

```sh
node -r portcheck-cli/register server.js
NODE_OPTIONS=--require=portcheck-cli/register npx next dev
```

```console
$ node -r portcheck-cli/register server.js
portcheck: port 3000 is in use by node (pid 41852), started by npm (41850) ← zsh (900).
portcheck: Stop it: npx portcheck-cli kill 3000
node:events:505
    throw er; // Unhandled 'error' event
    ^

Error: listen EADDRINUSE: address already in use :::3000
…
```

In `package.json`:

```json
{
  "scripts": {
    "dev": "node -r portcheck-cli/register server.js"
  }
}
```

The hook never changes what your program does: the error still reaches your own error
handlers, and an unhandled one still crashes with the same message and exit code.
It only adds the explanation, when the process exits with an error. If a server recovers
and listens on another port, as Vite and many frameworks do, the hook stays quiet.

## Which portcheck is used

Both use `PORTCHECK_BIN` if it's set, otherwise the portcheck this package has already
downloaded, otherwise `portcheck` on your `PATH`. The Vite plugin downloads it on first
use if none is found. A lookup that takes longer than 2 seconds, or a missing portcheck,
is skipped silently: the hook and the plugin never get in your dev server's way.

## Related

- [Run on a free port (`run`)](run.md): start any command on 3000 or the next free port
- [EADDRINUSE in Node.js, Next.js and Vite](../fix/node.md)
- [VS Code extension](vscode.md): the same explanation for commands in the terminal
