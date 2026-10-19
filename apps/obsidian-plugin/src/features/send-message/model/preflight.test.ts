/**
 * The corpus preflight: one question, asked before the first model call.
 *
 * Whether the corpus can be reached at all right now, and if it cannot, the
 * Spanish message that names the material and what unblocks the send (S6). The
 * probe is the corpus list, mocked at the module boundary so nothing touches the
 * network.
 */
import { beforeEach, describe, expect, it, vi } from "vitest"

const mocks = vi.hoisted(() => ({ listDocuments: vi.fn() }))

vi.mock("@/entities/document", () => ({ listDocuments: mocks.listDocuments }))

import { ApiError } from "@/shared/api/client"
import { checkCorpusReachable } from "./preflight"

const DOWN_ERROR = "No se pudo conectar al servidor. ¿Está corriendo en localhost:8080?"

const SINGLE_TITLE_MESSAGE =
  "No se pudo consultar «DDD»: el servidor del corpus no responde. " +
  "Encendé el servidor, o quitá ese adjunto para chatear sin él."

const SINGLE_TITLE_HTTP_ERROR_MESSAGE =
  "No se pudo consultar «DDD»: el servidor del corpus respondió con un error (HTTP 500). " +
  "Actualizá o revisá el servidor, o quitá ese adjunto para chatear sin él."

beforeEach(() => {
  vi.clearAllMocks()
  mocks.listDocuments.mockResolvedValue([])
})

describe("checkCorpusReachable", () => {
  it("lets the send through when the corpus answers", async () => {
    const decision = await checkCorpusReachable(["DDD"])

    expect(decision).toEqual({ ok: true })
    expect(mocks.listDocuments).toHaveBeenCalledTimes(1)
  })

  it("blocks the send and names the material when the corpus does not answer", async () => {
    mocks.listDocuments.mockRejectedValue(new Error(DOWN_ERROR))

    const decision = await checkCorpusReachable(["DDD"])

    // The question is "does it answer", not "why doesn't it": the cause the
    // server reported is not what the user reads.
    expect(decision).toEqual({ ok: false, error: SINGLE_TITLE_MESSAGE })
  })

  it("blocks on any thrown value, not only on an Error", async () => {
    mocks.listDocuments.mockRejectedValue("boom")

    const decision = await checkCorpusReachable(["DDD"])

    expect(decision).toEqual({ ok: false, error: SINGLE_TITLE_MESSAGE })
  })

  it("reads three titles as one Spanish list of attachments", async () => {
    mocks.listDocuments.mockRejectedValue(new Error(DOWN_ERROR))

    const decision = await checkCorpusReachable(["DDD", "SICP", "Refactoring"])

    expect(decision).toEqual({
      ok: false,
      error:
        "No se pudo consultar «DDD», «SICP» y «Refactoring»: el servidor del corpus no responde. " +
        "Encendé el servidor, o quitá esos adjuntos para chatear sin ellos.",
    })
  })

  it("caps the list at three titles and counts the rest", async () => {
    mocks.listDocuments.mockRejectedValue(new Error(DOWN_ERROR))

    const decision = await checkCorpusReachable(["DDD", "SICP", "Refactoring", "Clean Code"])

    expect(decision).toEqual({
      ok: false,
      error:
        "No se pudo consultar «DDD», «SICP», «Refactoring» y 1 más: " +
        "el servidor del corpus no responde. " +
        "Encendé el servidor, o quitá esos adjuntos para chatear sin ellos.",
    })
  })

  it("does not cache the probe: a server that came back up is seen as up", async () => {
    mocks.listDocuments.mockRejectedValueOnce(new Error(DOWN_ERROR))

    expect(await checkCorpusReachable(["DDD"])).toMatchObject({ ok: false })
    expect(await checkCorpusReachable(["DDD"])).toEqual({ ok: true })
    expect(mocks.listDocuments).toHaveBeenCalledTimes(2)
  })

  // The server answered, and said no. "No responde" would be false -- it did
  // answer -- and would send the user to start a server that is already running.
  it("names the status when the server answered with an error", async () => {
    mocks.listDocuments.mockRejectedValue(
      new ApiError("Error del servidor al sincronizar.", 500),
    )

    const decision = await checkCorpusReachable(["DDD"])

    expect(decision).toEqual({ ok: false, error: SINGLE_TITLE_HTTP_ERROR_MESSAGE })
  })

  it("names the status of a route an older server does not have", async () => {
    mocks.listDocuments.mockRejectedValue(new ApiError("not found", 404))

    const decision = await checkCorpusReachable(["DDD"])

    expect(decision).toEqual({
      ok: false,
      error:
        "No se pudo consultar «DDD»: el servidor del corpus respondió con un error (HTTP 404). " +
        "Actualizá o revisá el servidor, o quitá ese adjunto para chatear sin él.",
    })
  })

  // The distinction is the thrown value's class, not the presence of a status:
  // only `ApiError` means the server answered, so a bare error carrying a status
  // is still a transport failure.
  it("keeps the transport message for a status-shaped error that is not an ApiError", async () => {
    mocks.listDocuments.mockRejectedValue(
      Object.assign(new Error("Error del servidor al sincronizar."), { status: 500 }),
    )

    const decision = await checkCorpusReachable(["DDD"])

    expect(decision).toEqual({ ok: false, error: SINGLE_TITLE_MESSAGE })
  })

  it("forwards the signal to the probe and still decides by the answer", async () => {
    const { signal } = new AbortController()

    const decision = await checkCorpusReachable(["DDD"], signal)

    expect(decision).toEqual({ ok: true })
    expect(mocks.listDocuments).toHaveBeenCalledWith(signal)
  })
})

/** The error a probe rejects with, without caring how it was shaped. */
async function rejectionOf(probe: Promise<unknown>): Promise<unknown> {
  try {
    await probe
  } catch (e) {
    return e
  }

  throw new Error("the probe was expected to reject")
}

/**
 * Cancelling is the user's own decision, so the probe answers it instead of
 * deciding the corpus is unreachable: a block message would send them to start a
 * server that is fine.
 *
 * The probe answers it by rejecting with an `AbortError`, because the send path
 * already treats exactly that shape as a cancellation: it reports no error,
 * persists nothing and clears `sending`. A `{ ok: false }` decision could only be
 * told apart from a real block by widening the result type the caller reads.
 */
describe("checkCorpusReachable when the probe is cancelled", () => {
  const CANCELLED_MESSAGE = "La consulta del corpus se canceló."

  function abortError(): Error {
    const error = new Error(CANCELLED_MESSAGE)

    error.name = "AbortError"
    return error
  }

  it("answers the cancellation when the signal is already aborted", async () => {
    const controller = new AbortController()

    controller.abort()
    mocks.listDocuments.mockRejectedValue(abortError())

    const error = await rejectionOf(checkCorpusReachable(["DDD"], controller.signal))

    expect(error).toBeInstanceOf(Error)
    expect((error as Error).name).toBe("AbortError")
    expect((error as Error).message).not.toContain("no responde")
  })

  it("answers the cancellation even when the client reported it as a network failure", async () => {
    // What actually happens on the wire: `shared/api/client.ts` maps every
    // `fetch` failure -- an abort included -- to its own `NetworkError`, so the
    // thrown value cannot tell a cancel from a dead server. The signal can.
    const controller = new AbortController()

    controller.abort()
    mocks.listDocuments.mockRejectedValue(
      Object.assign(new Error(DOWN_ERROR), { name: "NetworkError" }),
    )

    const error = await rejectionOf(checkCorpusReachable(["DDD"], controller.signal))

    expect((error as Error).name).toBe("AbortError")
    expect((error as Error).message).not.toContain("no responde")
  })

  it("answers the cancellation when the signal aborts mid-flight", async () => {
    const controller = new AbortController()

    // A server that accepted the connection and never answers: the probe settles
    // only when its signal aborts.
    mocks.listDocuments.mockImplementation(
      (signal?: AbortSignal) =>
        new Promise((_resolve, reject) => {
          signal?.addEventListener("abort", () => reject(abortError()))
        }),
    )

    const probe = checkCorpusReachable(["DDD"], controller.signal)

    controller.abort()

    const error = await rejectionOf(probe)

    expect((error as Error).name).toBe("AbortError")
  })
})
