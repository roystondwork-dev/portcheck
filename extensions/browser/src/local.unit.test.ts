import { describe, expect, it } from "vitest";
import { localTarget } from "./local";

describe("localTarget", () => {
  it.each([
    ["http://localhost:3000/", "localhost:3000", 3000],
    ["http://127.0.0.1:8080/api?x=1", "127.0.0.1:8080", 8080],
    ["http://127.1.2.3:5000", "127.1.2.3:5000", 5000],
    ["http://[::1]:4000/", "[::1]:4000", 4000],
    ["http://0.0.0.0:9000", "0.0.0.0:9000", 9000],
    ["http://app.localhost:5173/", "app.localhost:5173", 5173],
    ["https://LOCALHOST:8443/", "localhost:8443", 8443],
    ["http://localhost/", "localhost:80", 80],
    ["https://localhost/", "localhost:443", 443],
  ])("%s", (url, label, port) => {
    expect(localTarget(url)).toEqual({ label, port });
  });

  it.each([undefined, "", "not a url", "https://example.com:3000/", "http://192.168.1.10:3000/", "file:///tmp/x.html", "chrome://extensions", "ws://localhost:3000/", "http://localhost.example.com:3000/"])(
    "ignores %j",
    (url) => {
      expect(localTarget(url)).toBeUndefined();
    },
  );
});
