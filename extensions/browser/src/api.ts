// The extension API: Firefox's promise-based `browser` namespace when present,
// otherwise Chrome's `chrome` (also promise-based in Manifest V3).
export const ext: typeof chrome = (globalThis as { browser?: typeof chrome }).browser ?? chrome;
