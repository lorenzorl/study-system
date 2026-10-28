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

/**
 * Where the preview server listens: `[host, port]`, declared once.
 *
 * Both sides of the readiness check have to agree, and they have to agree on
 * IPv4. The preview server defaults to the hostname `localhost`, which resolves
 * to the IPv6 loopback first, so a probe against the IPv4 address finds nothing
 * listening and the suite waits out its entire timeout instead of failing. The
 * address is therefore pinned rather than inherited.
 *
 * Host and port are separate values composed into a URL here, so the two places
 * that need the address cannot drift apart — and the port is a plain number, not
 * a licence plate buried in two string literals.
 */
const HOST = "127.0.0.1"
const PORT = 4173
const BASE_URL = `http://${HOST}:${PORT}`

export default defineConfig({
  testDir: "./e2e",
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 1 : 0,
  reporter: process.env.CI ? "github" : "list",
  snapshotPathTemplate: "{testDir}/__screenshots__/{testFileName}/{arg}-{platform}{ext}",
  use: {
    baseURL: BASE_URL,
    browserName: "chromium",
    viewport: { width: 1280, height: 800 },
    trace: "on-first-retry",
  },
  webServer: {
    // Build then serve, so the smoke exercises the same assets the Go binary
    // serves. `--strictPort` turns a busy port into a failure instead of a
    // silent second server on a different one.
    command: `pnpm build && pnpm preview --host ${HOST} --port ${PORT} --strictPort`,
    url: BASE_URL,
    // The visual net must never compare an artifact it did not just build. With
// `reuseExistingServer` on, any `vite preview` still listening on the port makes
// Playwright skip this whole command — build included — and the suite then
// compares the stale bundle against fresh baselines. It reported a false 12/12
// that way once. Together with `--strictPort` inside the command, a busy port is
// now a loud failure instead of a silent reuse.
reuseExistingServer: false,
    timeout: 180_000,
  },
})
