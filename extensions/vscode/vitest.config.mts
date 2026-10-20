import { defineConfig } from "vitest/config";

export default defineConfig({
  test: {
    include: ["src/**/*.unit.test.ts"],
    coverage: {
      provider: "v8",
      // Modules that import "vscode" are covered by the integration tests.
      include: ["src/cli.ts", "src/fixes.ts", "src/matcher.ts", "src/model.ts", "src/nodeExec.ts", "src/projectModel.ts"],
      thresholds: { lines: 100, functions: 100, branches: 100, statements: 100 },
    },
  },
});
