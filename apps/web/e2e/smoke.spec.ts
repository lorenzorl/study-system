import { expect, test, type Page } from "@playwright/test"

/**
 * How much of a page may differ before the comparison fails: nothing.
 *
 * It was 0.01 (1% of the page), and T6 proved what that costs. Migrating the
 * secondary actions changed their fill and added a shadow — about 0.4% of the
 * page — so the suite went green against a stale baseline, and the drift only
 * surfaced because the two images were compared by hand afterwards. A tolerance
 * wide enough to swallow a real change is a gate that reports "no change" for
 * changes it cannot see.
 *
 * `threshold` (Playwright's per-pixel colour distance, default 0.2) still absorbs
 * anti-aliasing noise; what is gone is the budget of pixels allowed to move.
 */
const PIXEL_TOLERANCE = 0

/**
 * The routes the migration touches, one screenshot each
 * (odd/tasks/shadcn-migration.md S3: "un smoke por ruta: 8 páginas, render + screenshot").
 */
const ROUTES = [
  { path: "/", name: "resources" },
  { path: "/topics", name: "topics" },
  { path: "/study/due", name: "due-cards" },
  { path: "/study/review", name: "review" },
  { path: "/topic/t1", name: "topic" },
  { path: "/topic/t1/concept/c1", name: "concept" },
  { path: "/topic/t1/concept/c1/feynman", name: "feynman" },
  { path: "/present/deck-atencion", name: "presentation" },
] as const

/**
 * The wire shapes, from the entity types: `ResourceResponse`, `TopicResponse` /
 * `ConceptDTO`, `DueCard`. Metrics are a hardcoded mock in
 * `entities/metrics/model/useMetricsStore.ts` and issue no request.
 */
const RESOURCES = [
  {
    id: "r1",
    title: "Atención selectiva",
    type: "note",
    source_uri: "estudio/atencion.md",
    topic_name: "Atención",
    created_at: "2026-10-01T10:00:00Z",
  },
  {
    id: "r2",
    title: "Sesgos cognitivos",
    type: "book",
    source_uri: "estudio/sesgos.pdf",
    topic_name: "Sesgos",
    created_at: "2026-10-02T10:00:00Z",
  },
]

const TOPICS = [
  {
    id: "t1",
    name: "Atención",
    concepts: [{ id: "c1", title: "Filtro atencional", file_path: "atencion/filtro.md" }],
  },
]

const DUE = [
  {
    flashcard_id: "f1",
    question: "¿Qué es el filtro atencional?",
    answer: "El mecanismo que selecciona qué entra en la conciencia.",
    concept_title: "Filtro atencional",
    topic_name: "Atención",
    next_review: "2026-10-20T10:00:00Z",
  },
]

/**
 * The middleware is not part of this harness: every `/api/**` call is answered
 * from the fixtures above, so a screenshot is deterministic and the suite runs in
 * CI with no Go binary. A page that only rendered its error state would not be a
 * visual regression test — it would be a screenshot of a failure.
 */
async function stubApi(page: Page): Promise<void> {
  await page.route("**/api/**", async (route) => {
    const request = route.request()

    if (request.method() !== "GET") {
      await route.fulfill({ status: 200, contentType: "application/json", body: "{}" })
      return
    }

    const { pathname } = new URL(request.url())
    const body =
      pathname === "/api/resources"
        ? RESOURCES
        : pathname === "/api/topics" || pathname === "/api/concepts"
          ? TOPICS
          : pathname === "/api/study/due"
            ? DUE
            : []

    await route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(body) })
  })
}

const CONSOLE_ERRORS: string[] = []

test.beforeEach(async ({ page }) => {
  CONSOLE_ERRORS.length = 0
  page.on("console", (message) => {
    if (message.type() === "error") CONSOLE_ERRORS.push(message.text())
  })
  page.on("pageerror", (error) => CONSOLE_ERRORS.push(error.message))

  await stubApi(page)
})

for (const route of ROUTES) {
  test(`renders ${route.name}`, async ({ page }) => {
    await page.goto(route.path)
    await page.waitForLoadState("networkidle")

    // The shell is always there; if this is empty the route did not mount.
    await expect(page.locator("#app")).not.toBeEmpty()

    expect(CONSOLE_ERRORS).toEqual([])

    await expect(page).toHaveScreenshot(`${route.name}.png`, {
      fullPage: true,
      animations: "disabled",
      maxDiffPixelRatio: PIXEL_TOLERANCE,
    })
  })
}

/**
 * The interactive states.
 *
 * The route captures above show each page as it first loads, and most of what the
 * primitive migration touches is behind a click: the new-topic form only exists
 * while its panel is open. A migration whose screenshot was green because the
 * component never rendered would be worse than no screenshot at all — this block
 * is the fix for that, and it grows by one case per migration unit, so every unit
 * is covered by the state it actually changes.
 */
test.describe("interactive states", () => {
  test("renders the new-topic form, open", async ({ page }) => {
    await page.goto("/topics")
    await page.getByRole("button", { name: "Agregar tema" }).click()
    await expect(page.getByPlaceholder(/Nombre del tema/)).toBeVisible()

    expect(CONSOLE_ERRORS).toEqual([])

    await expect(page).toHaveScreenshot("topics-new-topic-form.png", {
      fullPage: true,
      animations: "disabled",
      maxDiffPixelRatio: PIXEL_TOLERANCE,
    })
  })

  test("renders the sync-note dialog, open", async ({ page }) => {
    await page.goto("/topics")
    await page.getByRole("button", { name: "Sincronizar nota" }).click()
    await expect(page.getByRole("dialog")).toBeVisible()

    expect(CONSOLE_ERRORS).toEqual([])

    await expect(page).toHaveScreenshot("topics-sync-note-dialog.png", {
      fullPage: true,
      animations: "disabled",
      maxDiffPixelRatio: PIXEL_TOLERANCE,
    })
  })

  // Behaviour, not pixels, and the reason it is here: a template ref on a shared/ui
  // component is the component instance, not an element, so `SyncNoteTrigger` reads
  // `$el` to put focus back on the control the reader pressed. Nothing else in the
  // repo can see whether `$el` is that node — the unit suite runs without a DOM, and
  // the capture above never closes the panel — so this assertion is the only thing
  // that can tell.
  test("returns focus to the trigger when the sync-note panel closes", async ({ page }) => {
    await page.goto("/topics")

    const trigger = page.getByRole("button", { name: "Sincronizar nota" })
    const panel = page.getByRole("dialog", { name: "Sincronizar nota" })

    await trigger.focus()
    await trigger.click()
    await expect(panel).toBeVisible()

    await page.keyboard.press("Escape")
    await expect(panel).toBeHidden()

    // Polled rather than read once: the trigger hands focus back after
    // `await nextTick()`, which is a tick this test does not get to schedule.
    await expect
      .poll(() => trigger.evaluate((el) => el === document.activeElement))
      .toBe(true)

    expect(CONSOLE_ERRORS).toEqual([])
  })

  // The new-concept form only exists while the "+" toggle is on, so the topic
  // route capture above proves nothing about it: a green `topic.png` would also be
  // green if the form never mounted. This capture is the one that fails when the
  // migrated Input or Button regresses.
  //
  // It starts at `/topic/t1` directly, and that is half the assertion. Until D1 was
  // fixed this test had to walk in through `/topics` → topic card, because
  // `TopicPage` never loaded the topics store: a cold visit rendered "Tema no
  // encontrado." and the "+" toggle did not exist. The direct deep link is now the
  // discriminating check — it fails again the moment the mount-time load is
  // removed, which entering through `/topics` first would hide.
  test("renders the new-concept form, open", async ({ page }) => {
    await page.goto("/topic/t1")
    await page.getByRole("button", { name: "Agregar concepto" }).click()
    await expect(page.getByPlaceholder(/Título del concepto/)).toBeVisible()

    expect(CONSOLE_ERRORS).toEqual([])

    await expect(page).toHaveScreenshot("topic-new-concept-form.png", {
      fullPage: true,
      animations: "disabled",
      maxDiffPixelRatio: PIXEL_TOLERANCE,
    })
  })

  // Same reason: the capture panel is an overlay that does not exist until
  // "Agregar material" is clicked, so the resources route capture above is blind
  // to every control inside it.
  test("renders the capture panel, open", async ({ page }) => {
    await page.goto("/")
    await page.getByRole("button", { name: "Agregar material" }).click()
    await expect(page.getByRole("dialog", { name: "Agregar material" })).toBeVisible()

    expect(CONSOLE_ERRORS).toEqual([])

    await expect(page).toHaveScreenshot("resources-capture-panel.png", {
      fullPage: true,
      animations: "disabled",
      maxDiffPixelRatio: PIXEL_TOLERANCE,
    })
  })
})
