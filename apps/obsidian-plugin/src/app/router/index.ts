import { createRouter, createMemoryHistory } from "vue-router"
import { DashboardPage } from "@/pages/dashboard"
import { TopicPage } from "@/pages/topic"
import { ConceptPage } from "@/pages/concept"
import { FlashcardSessionPage } from "@/pages/flashcard-session"
import { FeynmanPage } from "@/pages/feynman"
import { DueCardsPage } from "@/pages/due-cards"

const routes = [
  {
    path: "/",
    name: "dashboard",
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
]

export const router = createRouter({
  history: createMemoryHistory(),
  routes,
})
