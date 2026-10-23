import type { ReviewCard } from "@/widgets/review-modal"

/**
 * Card content for the review modal, standing in for the endpoint that does not
 * exist yet (`GET /api/study/due` is the real source of a due session).
 *
 * This is **not** launcher data: the launcher's resources come from
 * `GET /api/resources`, so a material card never looks up a session here, and a
 * flashcard resource only appears once its own endpoint lands.
 */

// ─── Card fixtures for the review modal ──────────────────────────────────────

/**
 * Cards behind each flashcards resource, so `Repasar` opens a real session in
 * the browser harness. Card counts are fixture-sized, not the resource `total`:
 * the launcher's status line stays the source of truth for how much is due.
 */
const ATENCION_SELECTIVA_CARDS: ReviewCard[] = [
  {
    id: "as-1",
    question: "¿Qué es el filtro atencional?",
    answer:
      "El mecanismo que selecciona la información relevante de la escena y descarta el resto antes de que llegue a la memoria de trabajo.",
  },
  {
    id: "as-2",
    question: "¿Qué diferencia hay entre atención selectiva y atención dividida?",
    answer:
      "La selectiva filtra estímulos en competencia; la dividida reparte recursos entre dos tareas simultáneas.",
  },
  {
    id: "as-3",
    question: "¿Qué papel cumple la atención selectiva en la memoria de trabajo?",
    answer:
      "Es la puerta de entrada: la memoria de trabajo solo recibe lo que el filtro deja pasar, y su capacidad es limitada.",
  },
  {
    id: "as-4",
    question: "¿Qué es el efecto cóctel party?",
    answer:
      "La capacidad de seguir una conversación entre ruido de fondo y, a la vez, detectar el propio nombre en otra conversación.",
  },
  {
    id: "as-5",
    question: "¿Cómo se estudia la atención selectiva con el paradigma de flancos?",
    answer:
      "Se presenta un estímulo central rodeado de distractores congruentes o incongruentes; la diferencia de tiempos de respuesta mide el filtrado.",
  },
  {
    id: "as-6",
    question: "¿Qué es la ceguera por falta de atención?",
    answer:
      "No percibir un estímulo visible cuando la atención está comprometida en otra tarea.",
  },
  {
    id: "as-7",
    question: "¿Por qué la carga perceptiva modifica la distracción?",
    answer:
      "Con alta carga perceptiva el filtro descarta antes los distractores; con baja carga quedan recursos libres y distraen más.",
  },
  {
    id: "as-8",
    question: "¿Qué es el priming negativo?",
    answer:
      "El enlentecimiento al responder a un estímulo que antes fue ignorado: el filtro deja una huella que inhibe ese contenido.",
  },
]

const SESGOS_COGNITIVOS_CARDS: ReviewCard[] = [
  {
    id: "sc-1",
    question: "¿Qué es el sesgo de confirmación?",
    answer:
      "La tendencia a buscar, interpretar y recordar información que confirma las creencias previas, ignorando la que las contradice.",
  },
  {
    id: "sc-2",
    question: "¿Cómo se manifiesta el sesgo de disponibilidad al estimar riesgos?",
    answer:
      "Se juzga la probabilidad por la facilidad de recordar ejemplos: lo que apareció en las noticias se percibe como más frecuente.",
  },
  {
    id: "sc-3",
    question: "¿Qué es el anclaje y por qué persiste?",
    answer:
      "Ajustar una estimación a partir de un primer valor de referencia; persiste porque el ajuste posterior suele ser insuficiente.",
  },
]

const ATENCION_DIVIDIDA_CARDS: ReviewCard[] = [
  {
    id: "ad-1",
    question: "¿Qué es la atención dividida?",
    answer:
      "La asignación simultánea de recursos atencionales a dos o más tareas que compiten por el mismo sistema de procesamiento.",
  },
  {
    id: "ad-2",
    question: "¿Qué ocurre en doble tarea cuando ambas compiten por el mismo recurso?",
    answer:
      "Aparece interferencia: el rendimiento de una o de las dos tareas cae, aunque con práctica se reduce el costo.",
  },
]

const CARGA_COGNITIVA_CARDS: ReviewCard[] = [
  {
    id: "cc-1",
    question: "¿Qué es la carga cognitiva intrínseca?",
    answer:
      "La dificultad propia del contenido y de sus relaciones, que depende de cuántos elementos hay que sostener a la vez.",
  },
  {
    id: "cc-2",
    question: "¿Cómo reduce la carga extrínseca una buena segmentación?",
    answer:
      "Divide el material en pasos que caben en la memoria de trabajo y libera recursos para el aprendizaje en vez del formato.",
  },
]

/**
 * Keyed by resource id. The continue fixture points at the same cards as the
 * `atencion-selectiva` resource, because it is the same session resumed.
 */
const CARD_FIXTURES: Record<string, ReviewCard[]> = {
  "fc-atencion-selectiva": ATENCION_SELECTIVA_CARDS,
  "continue-atencion-selectiva": ATENCION_SELECTIVA_CARDS,
  "fc-sesgos-cognitivos": SESGOS_COGNITIVOS_CARDS,
  "fc-atencion-dividida": ATENCION_DIVIDIDA_CARDS,
  "fc-carga-cognitiva": CARGA_COGNITIVA_CARDS,
}

/** Cards for a review session; a resource with no fixture data opens empty. */
export function fixtureCardsFor(resourceId: string): ReviewCard[] {
  return CARD_FIXTURES[resourceId] ?? []
}
