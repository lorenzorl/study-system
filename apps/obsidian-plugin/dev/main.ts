import { createApp } from "vue"
import { createRouter, createWebHistory } from "vue-router"
import { createPinia } from "pinia"
import App from "../src/vue-app/App.vue"
import DashboardView from "../src/vue-app/views/DashboardView.vue"
import TopicView from "../src/vue-app/views/TopicView.vue"
import ConceptView from "../src/vue-app/views/ConceptView.vue"
import FlashcardSession from "../src/vue-app/views/FlashcardSession.vue"
import FeynmanModule from "../src/vue-app/views/FeynmanModule.vue"

const routes = [
  { path: "/", name: "dashboard", component: DashboardView },
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

function init() {
  const router = createRouter({
    history: createWebHistory("/"),
    routes,
  })

  const app = createApp(App)
  app.use(createPinia())
  app.use(router)
  app.mount("#app")
}

if (document.readyState === "loading") {
  document.addEventListener("DOMContentLoaded", init)
} else {
  init()
}
