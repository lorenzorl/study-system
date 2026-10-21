import { createRouter, createMemoryHistory } from "vue-router"
import { ResourcesPage } from "@/pages/resources"
import { TopicPage } from "@/pages/topic"
import { ConceptPage } from "@/pages/concept"
import { FlashcardSessionPage } from "@/pages/flashcard-session"
import { FeynmanPage } from "@/pages/feynman"
import { DueCardsPage } from "@/pages/due-cards"
import { PresentationPage } from "@/pages/presentation"

const routes = [
  {
    path: "/",
    name: "dashboard",
    component: ResourcesPage,
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
  // The deck tab (§3). Reveal's stylesheets reach this page as text and land in
  // the deck's shadow root, so the page can live in the plugin graph.
  {
    path: "/present/:deckId",
    name: "presentation",
    component: PresentationPage,
  },
]

export const router = createRouter({
  history: createMemoryHistory(),
  routes,
})
