---
title: "Port 8081 already in use (React Native Metro, Expo) and adb port 5037"
description: "Fix “EADDRINUSE: address already in use :::8081” in React Native and Expo, and adb’s “cannot bind to 127.0.0.1:5037”: find the old Metro bundler or adb server and stop it."
---

# Port 8081 already in use (React Native, Expo, adb)

## React Native: Metro on port 8081

```text
error listen EADDRINUSE: address already in use :::8081
```

or, from older versions of the React Native CLI:

```text
Port 8081 already in use, packager is either not running or not running correctly
```

Metro, the React Native bundler, listens on port 8081. The holder is almost always
**an earlier Metro** still running in another terminal, or one that the iOS or Android
build started in its own window.

```sh
npx portcheck-cli kill 8081
```

```console
About to terminate:
  PID 51234  node  (port 8081), started by npm (51200) ← zsh (900)
Proceed? [y/N] y
PID    PROCESS  RESULT      DETAIL
51234  node     terminated
```

Then start Metro again: `npx react-native start`.

To run two projects at once instead, start the second Metro on another port with
`npx react-native start --port 8082`. The app must then be built to use that port too,
which is more work than stopping the old Metro.

## Expo

Expo's dev server uses port **8081** from SDK 49 on (older projects, with the classic
`expo-cli`, used 19000 to 19002). When the port is busy, Expo asks whether to use the
next one. Saying yes works for a quick test, but if you expected your old server to
be gone, stop it:

```sh
npx portcheck-cli kill 8081
```

Or choose the port yourself: `npx expo start --port 8082`.

## Android: adb on port 5037

```text
could not install *smartsocket* listener: cannot bind to 127.0.0.1:5037: Address already in use
```

The adb server listens on port 5037, and only one can. This error usually means **two
different adb versions**: Android Studio's and another one on your `PATH` (Homebrew,
a separate platform-tools download, or a tool that bundles its own adb). Each one kills
and restarts the other's server.

```sh
portcheck 5037 --json   # which adb holds it: "exe" is the path of that copy
adb kill-server
adb start-server
```

The lasting fix is to use one adb everywhere: put the Android SDK's
`platform-tools` first on your `PATH`, or remove the extra copy.

## Related

- [Node.js, Next.js, Vite, React](node.md)
- [macOS](macos.md) · [Windows](windows.md) · [Linux](linux.md)
- [All "port already in use" fixes](index.md)
