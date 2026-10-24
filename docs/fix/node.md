---
title: "EADDRINUSE: address already in use :::3000 (Node.js, Next.js, Vite, React)"
description: "Fix “Error: listen EADDRINUSE: address already in use” in Node.js, Express, Next.js, Vite and React: find the old dev server holding the port and stop it, or start on a free port."
---

# EADDRINUSE: address already in use (Node.js)

```text
Error: listen EADDRINUSE: address already in use :::3000
    at Server.setupListenHandle [as _listen2] (node:net:...)
  code: 'EADDRINUSE',
  errno: -48,
  syscall: 'listen',
  port: 3000
```

Another process is listening on port 3000. Nine times out of ten it's **your own dev
server from earlier**, left running in another terminal tab, a closed editor, or after
++ctrl+z++ suspended it.

## Fix it

```sh
npx portcheck-cli kill 3000
```

```console
About to terminate:
  PID 42379  node  (port 3000), started by npm (4200) ← zsh (900)
Proceed? [y/N] y
PID    PROCESS  RESULT      DETAIL
42379  node     terminated
```

The `started by` chain tells you whether it's an old `npm run dev`, a server your editor
launched (`Code Helper`), or something else. portcheck stops it gently first (`SIGTERM`)
so it can clean up, and never touches system processes.

To just see what's there: `npx portcheck-cli 3000`.

## Vite, Next.js and others that switch ports

Some dev servers don't fail; they quietly move to the next port:

```text
Port 5173 is in use, trying another one...
```

Your app then runs on 5174, which breaks bookmarks, OAuth callback URLs and CORS
settings that expect 5173. Free the usual port with `npx portcheck-cli kill 5173`, or
make Vite fail instead of switching with `server.strictPort: true`.

## Never see it again

Start your dev server through `portcheck run`. It uses your port when it's free, and
otherwise either tells you who holds it or picks the next free one:

```sh
# Explain who holds 3000 and stop, instead of a stack trace:
npx portcheck-cli run --port 3000 -- npm run dev

# Or move to the next free port and pass it in $PORT:
npx portcheck-cli run --port 3000 --next -- npm run dev
```

Most Node frameworks read `$PORT` (Express apps usually do
`app.listen(process.env.PORT || 3000)`; Next.js and Create React App read it directly).
For tools that take a flag, use `{port}`:
`portcheck run --port 5173 --next -- npx vite --port {port}`.

In `package.json`:

```json
{
  "scripts": {
    "dev": "portcheck run --port 3000 --next -- next dev"
  },
  "devDependencies": {
    "portcheck-cli": "^0.10.0"
  }
}
```

## Editor and browser

- **VS Code:** when a command in the integrated terminal fails with `EADDRINUSE`, the
  [portcheck extension](../guide/vscode.md) names the process, stops it and runs your
  command again.
- **JetBrains IDEs** (WebStorm, IntelliJ): the [plugin](../guide/jetbrains.md) does the
  same for run configurations.
- **Browser:** the [extension](../guide/browser.md) shows which process serves each
  `localhost` tab.

## Related

- [macOS](macos.md) · [Windows](windows.md) · [Linux](linux.md)
- [portcheck vs kill-port](kill-port.md)
- [All "port already in use" fixes](index.md)
