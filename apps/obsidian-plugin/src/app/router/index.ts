import { createRouter, createMemoryHistory } from "vue-router"
import DashboardView from "@/vue-app/views/DashboardView.vue"
import TopicView from "@/vue-app/views/TopicView.vue"
import ConceptView from "@/vue-app/views/ConceptView.vue"
import FlashcardSession from "@/vue-app/views/FlashcardSession.vue"
import FeynmanModule from "@/vue-app/views/FeynmanModule.vue"
import DueCardsView from "@/vue-app/views/DueCardsView.vue"

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
