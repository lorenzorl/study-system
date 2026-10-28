import { defineConfig } from "@playwright/test"

/**
 * The visual safety net for the shared/ui migration (odd/tasks/shadcn-migration.md T1).
 *
 * It runs against the **production bundle** (`vite preview`), not the dev server:
 * a screenshot has to show what ships, and the dev server injects an HMR client
 * and un-minified CSS that would make the baseline meaningless.
 *
 * `snapshotPathTemplate` carries `{platform}` on purpose. Font rasterisation
 * differs between macOS and Linux, so a baseline captured on one fails on the
 * other for reasons that have nothing to do with the code. Each platform keeps
 * its own baselines; see the note in ARCHITECTURE §3.2 about the Linux set.
 */
export default defineConfig({
  testDir: "./e2e",
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 1 : 0,
  reporter: process.env.CI ? "github" : "list",
  snapshotPathTemplate: "{testDir}/__screenshots__/{testFileName}/{arg}-{platform}{ext}",
  use: {
    baseURL: "http://127.0.0.1:4173",
    browserName: "chromium",
    viewport: { width: 1280, height: 800 },
    trace: "on-first-retry",
  },
  webServer: {
    // Build then serve, so the smoke exercises the same assets the Go binary serves.
    //
    // `--host 127.0.0.1` is not cosmetic: `vite preview` defaults to `localhost`,
    // which on this machine binds **IPv6 loopback only** (`[::1]:4173`, verified
    // with `lsof`). A readiness check against `127.0.0.1` then connects to nothing
    // and the suite dies after the full timeout. Pinning both sides to IPv4 is
    // deterministic on every platform.
    command: "pnpm build && pnpm preview --host 127.0.0.1 --port 4173 --strictPort",
    url: "http://127.0.0.1:4173",
    reuseExistingServer: !process.env.CI,
    timeout: 180_000,
  },
})
