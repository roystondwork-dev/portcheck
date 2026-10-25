import type { Plugin } from "vite";

/** Says who holds your dev server's port when Vite moves to another one, and adds `k + enter` to stop it and restart. */
interface Portcheck {
  (): Plugin;
  default: Portcheck;
  portcheck: Portcheck;
}

declare const portcheck: Portcheck;
export = portcheck;
