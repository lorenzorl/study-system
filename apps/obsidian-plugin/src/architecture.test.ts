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
 * outside `app/` and `shared/obsidian/` imports `obsidian` (§2, §3, §4, §9).
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

/** §4: only these two places may import the Obsidian package. */
const OBSIDIAN_ALLOWED_PREFIXES = ["app/", "shared/obsidian/"]

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

function scan(files: string[]): Violation[] {
  const violations: Violation[] = []

  for (const file of files) {
    const ownUnit = unitOf(file)
    const [ownLayer] = file.split("/") as [Layer | undefined]
    const source = readFileSync(path.join(SRC, file), "utf8")

    for (const specifier of importsOf(source)) {
      if (specifier === "obsidian") {
        if (!OBSIDIAN_ALLOWED_PREFIXES.some((prefix) => file.startsWith(prefix))) {
          violations.push({ file, specifier, rule: "obsidian-boundary" })
        }
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
        "app/study-chat-view.ts",
        "pages/chat/ui/ChatPage.vue",
        "pages/corpus/ui/CorpusPage.vue",
        "widgets/chat-thread/ui/ChatThread.vue",
        "features/send-message/model/prompt.ts",
        "features/sync-note/model/useSyncNote.ts",
        "entities/chat/lib/note.ts",
        "entities/topic/model/types.ts",
        "shared/api/client.ts",
        "shared/config/settings.ts",
        "shared/llm/client.ts",
        "shared/obsidian/useObsidian.ts",
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

  it("imports obsidian only from app/ and shared/obsidian/", () => {
    expect(report("obsidian-boundary")).toEqual([])
  })

  it("keeps files outside the layers out of the layer graph", () => {
    expect(report("unlayered-module")).toEqual([])
  })
})
