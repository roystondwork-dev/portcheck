// The Go CLI's golden JSON output, so these tests fail if the CLI's JSON changes.
import { readFileSync } from "node:fs";
import * as path from "node:path";
import type { CheckData, Envelope, ListData } from "./schema";

function golden<T>(name: string): Envelope<T> {
  const file = path.join(__dirname, "..", "..", "..", "internal", "output", "testdata", `${name}.json.golden`);
  return JSON.parse(readFileSync(file, "utf8")) as Envelope<T>;
}

export const checkGolden = golden<CheckData>("check");
export const listGolden = golden<ListData>("list");
