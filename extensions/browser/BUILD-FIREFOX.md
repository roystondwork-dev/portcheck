# Building the Firefox add-on from source

For addons.mozilla.org reviewers. This archive is the `extensions/browser` folder of
https://github.com/roystondwork-dev/portcheck at the release's tag; it builds the
submitted add-on exactly.

## Requirements

- Any OS with **Node.js 22 or later** and **npm 11 or later** (tested with the reviewers'
  default: Ubuntu 24.04, Node 24, npm 11).
- Network access for `npm ci` (packages pinned in `package-lock.json`).

## Build

```sh
npm ci
npm run build:firefox
```

The add-on is in `dist-firefox/` (`manifest.json`, `background.js`, `popup.js`,
`popup.html`, `popup.css`, `icons/`). The JavaScript is bundled from `src/*.ts` with
esbuild (pinned in the lockfile) and is **not minified** for Firefox. `esbuild.mjs`
writes the Firefox manifest from `static/manifest.json`.

## What the add-on does

It reads the address of localhost tabs only and talks to the user's locally installed
portcheck program through native messaging (host
`io.github.roystondwork_dev.portcheck`). It makes no network requests and collects no
data. Testing steps are in the listing's notes for reviewers.
