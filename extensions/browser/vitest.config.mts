import { defineConfig } from "vitest/config";

export default defineConfig({
  test: {
    include: ["src/**/*.unit.test.ts"],
    coverage: {
      provider: "v8",
      // background.ts and popup.ts use the chrome.* and DOM APIs; the end-to-end
      // tests in e2e/ cover them in a real browser.
      include: ["src/host.ts", "src/local.ts", "src/model.ts", "src/view.ts"],
      thresholds: { lines: 100, functions: 100, branches: 100, statements: 100 },
    },
  },
});
