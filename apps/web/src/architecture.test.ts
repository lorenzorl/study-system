import { readdirSync, readFileSync } from "node:fs"
import path from "node:path"
import { fileURLToPath } from "node:url"
import { describe, expect, it } from "vitest"

/**
 * ARCHITECTURE §10 — the boundary guard.
 *
 * This suite replaces the ESLint option so the repository adds no dependency.
 * It walks `src/`, parses every import, and fails when an import points upward,
 * to a sibling slice of the same layer, into a slice's internals instead of its
 * `index.ts`, out of the file's own slice through a relative path, or when a file
 * imports `obsidian` at all: this app has no vault (§2, §3, §4, §9).
 */

const SRC = path.dirname(fileURLToPath(import.meta.url))
const SOURCE_EXTENSIONS = [".ts", ".vue"]

/** Higher layer = smaller rank. An import may only target a strictly greater rank. */
const LAYERS = ["app", "pages", "widgets", "features", "entities", "shared"] as const
type Layer = (typeof LAYERS)[number]

const RANK: Record<Layer, number> = {
  app: 0,
  pages: 1,
  widgets: 2,
  features: 3,
  entities: 4,
  shared: 5,
}

type Rule =
  | "layer-direction"
  | "same-layer-sibling"
  | "public-api"
  | "relative-escape"
  | "obsidian-boundary"
  | "unlayered-module"

interface Violation {
  file: string
  specifier: string
  rule: Rule
}

function toPosix(value: string): string {
  return value.split(path.sep).join("/")
}

/** Every `.ts` and `.vue` file under `src/`, as `src`-relative posix paths. */
function collectFiles(dir: string): string[] {
  const found: string[] = []

  for (const entry of readdirSync(dir, { withFileTypes: true })) {
    const full = path.join(dir, entry.name)

    if (entry.isDirectory()) {
      found.push(...collectFiles(full))
      continue
    }

    if (SOURCE_EXTENSIONS.some((extension) => entry.name.endsWith(extension))) {
      found.push(toPosix(path.relative(SRC, full)))
    }
  }

  return found.sort()
}

/** Every module specifier in a source file, whatever the import or export form. */
function importsOf(source: string): string[] {
  const patterns = [
    /\bfrom\s+["']([^"']+)["']/g,
    /\bimport\s*\(\s*["']([^"']+)["']\s*\)/g,
    /^\s*import\s+["']([^"']+)["']/gm,
  ]
  const found = new Set<string>()

  for (const pattern of patterns) {
    for (const match of source.matchAll(pattern)) {
      found.add(match[1])
    }
  }

  return [...found]
}

function isLayer(segment: string | undefined): segment is Layer {
  return (LAYERS as readonly string[]).includes(segment ?? "")
}

/**
 * The unit a file belongs to: `pages/dashboard` for a slice, `app` for the
 * integration layer, `shared/<segment>` for shared (which has no slices), and
 * `null` for a file that lives outside the layer system.
 */
function unitOf(file: string): string | null {
  const [layer, second] = file.split("/")

  if (!isLayer(layer)) return null
  if (layer === "app") return "app"
  if (!second) return null

  return `${layer}/${second}`
}

interface ScannedFile {
  file: string
  specifiers: string[]
}

/**
 * The rule engine. It takes specifiers, not source text: a fixture that spells
 * out an import statement inside a string literal is data, and reading it back
 * as an import of the file that holds it is how a guard starts lying about
 * itself — including when the statement appears in a comment.
 */
function scanSpecifiers(entries: ScannedFile[]): Violation[] {
  const violations: Violation[] = []

  for (const { file, specifiers } of entries) {
    const ownUnit = unitOf(file)
    const [ownLayer] = file.split("/") as [Layer | undefined]

    for (const specifier of specifiers) {
      if (specifier === "obsidian") {
        violations.push({ file, specifier, rule: "obsidian-boundary" })
        continue
      }

      const isAlias = specifier.startsWith("@/")
      const isRelative = specifier.startsWith(".")

      // A package import (vue, pinia, node:fs, …) is not a layer concern.
      if (!isAlias && !isRelative) continue

      if (isAlias) {
        const parts = specifier.slice(2).split("/")
        const [targetLayer, targetSliceName] = parts

        if (!isLayer(targetLayer)) {
          violations.push({ file, specifier, rule: "public-api" })
          continue
        }

        if (!ownUnit || !ownLayer) {
          violations.push({ file, specifier, rule: "unlayered-module" })
          continue
        }

        if (targetLayer === ownLayer) {
          // `shared` has no slices (§2): its units are segments, and one segment
          // reaches another by its public path (`@/shared/lib/utils`), which is
          // exactly the shape the downward branch already accepts. The
          // same-layer rule is about *slices* — a sibling slice of `entities`,
          // `features`, `widgets` or `pages` — and applying it to `shared` would
          // forbid a segment from using another segment at all, leaving every
          // `shared/ui` primitive unable to import `cn`. Only the shallow
          // `@/shared` form stays rejected, because it names no segment.
          if (ownLayer === "shared") {
            if (parts.length < 2) {
              violations.push({ file, specifier, rule: "public-api" })
            }
            continue
          }

          const targetUnit =
            targetLayer === "app" ? "app" : `${targetLayer}/${targetSliceName ?? ""}`
          violations.push({
            file,
            specifier,
            // §9: a slice may import only `index.ts`; its own module it reaches
            // with a relative path.
            rule: targetUnit === ownUnit ? "public-api" : "same-layer-sibling",
          })
          continue
        }

        if (RANK[targetLayer] < RANK[ownLayer]) {
          violations.push({ file, specifier, rule: "layer-direction" })
          continue
        }

        // A lower layer is reachable only through a slice root; `shared` has no
        // slices, so its segments (`shared/api/client`) are the public paths.
        const isSliceRoot = parts.length === 2
        const isSharedModule = targetLayer === "shared" && parts.length >= 2

        if (!isSliceRoot && !isSharedModule) {
          violations.push({ file, specifier, rule: "public-api" })
        }
        continue
      }

      // A relative import may not leave the file's own unit.
      const target = path.posix.normalize(
        path.posix.join(path.posix.dirname(file), specifier),
      )
      const targetUnit = target.startsWith("..") ? null : unitOf(target)

      if (!ownUnit) {
        if (targetUnit || target.startsWith("..")) {
          violations.push({ file, specifier, rule: "unlayered-module" })
        }
        continue
      }

      if (targetUnit !== ownUnit) {
        violations.push({ file, specifier, rule: "relative-escape" })
      }
    }
  }

  return violations
}

function scan(files: string[]): Violation[] {
  return scanSpecifiers(
    files.map((file) => ({
      file,
      specifiers: importsOf(readFileSync(path.join(SRC, file), "utf8")),
    })),
  )
}

const FILES = collectFiles(SRC)
const VIOLATIONS = scan(FILES)

function report(rule: Rule): string[] {
  return VIOLATIONS.filter((violation) => violation.rule === rule).map(
    (violation) => `${violation.file} -> ${violation.specifier}`,
  )
}

describe("FSD boundaries", () => {
  it("walks the source tree instead of passing vacuously", () => {
    expect(FILES).toEqual(
      expect.arrayContaining([
        "app/main.ts",
        "pages/dashboard/ui/DashboardPage.vue",
        "widgets/topic-list/ui/TopicList.vue",
        "features/create-topic/model/useCreateTopic.ts",
        "features/sync-note/model/useSyncNote.ts",
        "entities/flashcard/lib/markdown-parser.ts",
        "entities/topic/model/types.ts",
        "shared/api/client.ts",
        "shared/config/settings.ts",
        "shared/lib/guards.ts",
        "app/router/index.ts",
      ]),
    )
  })

  it("keeps every import pointing to a strictly lower layer", () => {
    expect(report("layer-direction")).toEqual([])
  })

  it("keeps slices of the same layer independent", () => {
    expect(report("same-layer-sibling")).toEqual([])
  })

  it("reaches other slices through their index.ts only", () => {
    expect(report("public-api")).toEqual([])
  })

  it("keeps relative imports inside the file's own slice", () => {
    expect(report("relative-escape")).toEqual([])
  })

  it("never imports the obsidian package", () => {
    expect(report("obsidian-boundary")).toEqual([])
  })

  it("keeps files outside the layers out of the layer graph", () => {
    expect(report("unlayered-module")).toEqual([])
  })
})

/**
 * The suite above proves the real tree is clean. That is only half a guard: a
 * rule engine that silently stopped matching would report no violations and
 * look perfect. These cases hand `scanSpecifiers` one synthetic file each and
 * assert the rule still fires — and, for `shared`, that it fires in exactly one
 * direction.
 *
 * They pass bare specifiers, not source text: a fixture written as a whole
 * import statement would be read back by this file's own scan as a real import
 * of `architecture.test.ts`, which sits outside the layers and may import none.
 */
function violationsIn(entries: Array<[file: string, specifier: string]>): string[] {
  return scanSpecifiers(entries.map(([file, specifier]) => ({ file, specifiers: [specifier] }))).map(
    (violation) => `${violation.rule}: ${violation.specifier}`,
  )
}

describe("the boundary guard still rejects what it must", () => {
  it("an upward import", () => {
    expect(violationsIn([["pages/dashboard/ui/Page.vue", "@/app/ui/App.vue"]])).toEqual([
      "layer-direction: @/app/ui/App.vue",
    ])
  })

  it("a sibling slice of the same layer", () => {
    expect(violationsIn([["entities/topic/model/types.ts", "@/entities/concept"]])).toEqual([
      "same-layer-sibling: @/entities/concept",
    ])
  })

  it("a deep import into another slice's segment", () => {
    expect(
      violationsIn([
        ["pages/topic/ui/TopicPage.vue", "@/features/select-topic/model/useTopicSelection"],
      ]),
    ).toEqual(["public-api: @/features/select-topic/model/useTopicSelection"])
  })

  it("a relative import that leaves the file's own unit", () => {
    expect(
      violationsIn([["features/create-topic/model/a.ts", "../../create-concept/model/b"]]),
    ).toEqual(["relative-escape: ../../create-concept/model/b"])
  })

  it("the obsidian package", () => {
    expect(violationsIn([["entities/topic/model/a.ts", "obsidian"]])).toEqual([
      "obsidian-boundary: obsidian",
    ])
  })

  it("a file outside the layers importing into them", () => {
    expect(violationsIn([["shims-vue.d.ts", "@/shared/lib/guards"]])).toEqual([
      "unlayered-module: @/shared/lib/guards",
    ])
  })
})

describe("shared has segments, not slices", () => {
  it("lets one segment reach another by its public path", () => {
    expect(violationsIn([["shared/ui/button/Button.vue", "@/shared/lib/utils"]])).toEqual([])
  })

  it("still rejects the shallow `@/shared` form, which names no segment", () => {
    expect(violationsIn([["shared/ui/button/Button.vue", "@/shared"]])).toEqual([
      "public-api: @/shared",
    ])
  })
})
