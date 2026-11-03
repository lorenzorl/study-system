import { createRouter, createWebHistory } from "vue-router"
import { ResourcesPage } from "@/pages/resources"
import { TodayPage } from "@/pages/today"
import { DashboardPage } from "@/pages/dashboard"
import { TopicPage } from "@/pages/topic"
import { ConceptPage } from "@/pages/concept"
import { FlashcardSessionPage } from "@/pages/flashcard-session"
import { FeynmanPage } from "@/pages/feynman"
import { DueCardsPage } from "@/pages/due-cards"
import { PresentationPage } from "@/pages/presentation"
import { ExplainPage } from "@/pages/explain"

/**
 * The app's routes.
 *
 * `/` is the resource launcher, not the topic dashboard: the launcher is what a
 * person opens the app to see (`PRESENTATION-UI.md` §3), and the topics page is
 * one destination away.
 *
 * Web history, not memory history: the browser owns the URL, a deep link to a
 * deck or a concept has to survive a reload, and the Go server answers every
 * unknown path with `index.html` so the client can resolve it (see
 * `internal/server` in the middleware).
 */
const routes = [
  {
    path: "/",
    name: "dashboard",
    component: ResourcesPage,
  },
  // What to do today, reached from the launcher's own control row: the projection
  // is the answer to the question the launcher only gestures at (S13, S14).
  {
    path: "/today",
    name: "today",
    component: TodayPage,
  },
  // Explain one page range in your own words and read the gaps it left. The URL
  // names the document and the range, and `?id=` names the stored explanation, so a
  // reload re-reads the gaps instead of paying for the model again (S9). It is
  // reachable by URL only until `/today` links to it (U4).
  {
    path: "/explain/:documentId/:from/:to",
    name: "explain",
    component: ExplainPage,
  },
  {
    path: "/topics",
    name: "topics",
    component: DashboardPage,
  },
  {
    path: "/study/due",
    name: "due-cards",
    component: DueCardsPage,
  },
  {
    path: "/study/review",
    name: "review",
    component: FlashcardSessionPage,
  },
  {
    path: "/topic/:topicId",
    name: "topic",
    component: TopicPage,
    props: true,
  },
  {
    path: "/topic/:topicId/concept/:conceptId",
    name: "concept",
    component: ConceptPage,
    props: true,
  },
  {
    path: "/topic/:topicId/concept/:conceptId/flashcards",
    name: "flashcards",
    component: FlashcardSessionPage,
    props: true,
  },
  {
    path: "/topic/:topicId/concept/:conceptId/feynman",
    name: "feynman",
    component: FeynmanPage,
    props: true,
  },
  // Reveal's stylesheets reach this page as text and land in the deck's shadow
  // root, so the page lives in the app graph like any other.
  {
    path: "/present/:deckId",
    name: "presentation",
    component: PresentationPage,
  },
]

export const router = createRouter({
  history: createWebHistory("/"),
  routes,
})
