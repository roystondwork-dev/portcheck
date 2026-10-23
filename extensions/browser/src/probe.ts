// Without the portcheck app, the extension can still tell whether a local
// page's server is up: by connecting to it.

/** fetch, injectable for tests. */
export type Fetch = (url: string, init: RequestInit) => Promise<unknown>;

/**
 * Whether something answers HTTP at url. Any answer counts, even an error
 * status; a refused connection or no answer within timeoutMs doesn't.
 */
export async function isUp(fetchFn: Fetch, url: string, timeoutMs = 2000): Promise<boolean> {
  try {
    await fetchFn(url, { method: "HEAD", cache: "no-store", credentials: "omit", redirect: "manual", signal: AbortSignal.timeout(timeoutMs) });
    return true;
  } catch {
    return false;
  }
}
