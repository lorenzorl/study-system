import { createRouter, createMemoryHistory } from "vue-router"
import DashboardView from "../views/DashboardView.vue"
import TopicView from "../views/TopicView.vue"
import ConceptView from "../views/ConceptView.vue"
import FlashcardSession from "../views/FlashcardSession.vue"
import FeynmanModule from "../views/FeynmanModule.vue"
import DueCardsView from "../views/DueCardsView.vue"

const routes = [
  {
    path: "/",
    name: "dashboard",
    component: DashboardView,
  },
  {
    path: "/study/due",
    name: "due-cards",
    component: DueCardsView,
  },
  {
    path: "/study/review",
    name: "review",
    component: FlashcardSession,
  },
  {
    path: "/topic/:topicId",
    name: "topic",
    component: TopicView,
    props: true,
  },
  {
    path: "/topic/:topicId/concept/:conceptId",
    name: "concept",
    component: ConceptView,
    props: true,
  },
  {
    path: "/topic/:topicId/concept/:conceptId/flashcards",
    name: "flashcards",
    component: FlashcardSession,
    props: true,
  },
  {
    path: "/topic/:topicId/concept/:conceptId/feynman",
    name: "feynman",
    component: FeynmanModule,
    props: true,
  },
]

export const router = createRouter({
  history: createMemoryHistory(),
  routes,
})
