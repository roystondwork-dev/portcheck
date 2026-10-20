// Which pages are served from this machine.

export interface LocalTarget {
  /** e.g. "localhost:3000" */
  label: string;
  port: number;
}

const LOOPBACK = /^(localhost|.+\.localhost|127(\.\d{1,3}){3}|\[::1\]|0\.0\.0\.0)$/;

/** Returns the port of an http(s) URL on this machine, or undefined. */
export function localTarget(url: string | undefined): LocalTarget | undefined {
  if (url === undefined) {
    return undefined;
  }
  let u: URL;
  try {
    u = new URL(url);
  } catch {
    return undefined;
  }
  if ((u.protocol !== "http:" && u.protocol !== "https:") || !LOOPBACK.test(u.hostname.toLowerCase())) {
    return undefined;
  }
  const port = u.port === "" ? (u.protocol === "https:" ? 443 : 80) : Number(u.port);
  return { label: `${u.hostname}:${port}`, port };
}
