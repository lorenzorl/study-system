import { createApp } from "vue"
import { createRouter, createWebHistory } from "vue-router"
import { pinia } from "@/app/pinia"
import App from "@/app/ui/App.vue"
import { ResourcesPage } from "@/pages/resources"
import { TopicPage } from "@/pages/topic"
import { ConceptPage } from "@/pages/concept"
import { FlashcardSessionPage } from "@/pages/flashcard-session"
import { FeynmanPage } from "@/pages/feynman"

const routes = [
  { path: "/", name: "dashboard", component: ResourcesPage },
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

function init() {
  const router = createRouter({
    history: createWebHistory("/"),
    routes,
  })

  const app = createApp(App)
  app.use(pinia)
  app.use(router)
  app.mount("#app")
}

if (document.readyState === "loading") {
  document.addEventListener("DOMContentLoaded", init)
} else {
  init()
}
