import { defineStore } from "pinia"
import { ref } from "vue"
import type { UploadDocumentResponse } from "@/entities/document"
import { uploadDocument } from "@/entities/document"

/**
 * The upload interaction: send one file to the corpus and keep the accepted
 * document so the UI can report its id and initial state. Polling the ingestion
 * status is a separate interaction (`entities/document`'s `fetchDocumentStatus`)
 * and is not driven from here.
 */
export const useUploadDocument = defineStore("upload-document", () => {
  const uploading = ref(false)
  const error = ref<string | null>(null)
  const uploaded = ref<UploadDocumentResponse | null>(null)

  async function upload(file: File): Promise<void> {
    uploading.value = true
    error.value = null
    uploaded.value = null

    try {
      uploaded.value = await uploadDocument(file)
    } catch (e) {
      error.value =
        e instanceof Error ? e.message : "No se pudo subir el documento."
    } finally {
      uploading.value = false
    }
  }

  function clearError(): void {
    error.value = null
  }

  return {
    uploading,
    error,
    uploaded,
    upload,
    clearError,
  }
})
