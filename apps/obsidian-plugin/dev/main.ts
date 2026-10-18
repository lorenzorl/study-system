import { createApp } from "vue"
import { createRouter, createWebHistory } from "vue-router"
import { createPinia } from "pinia"
import App from "@/app/ui/App.vue"
import { DashboardPage } from "@/pages/dashboard"
import { TopicPage } from "@/pages/topic"
import { ConceptPage } from "@/pages/concept"
import { FlashcardSessionPage } from "@/pages/flashcard-session"
import { FeynmanPage } from "@/pages/feynman"
import { CorpusPage } from "@/pages/corpus"

const routes = [
  { path: "/", name: "dashboard", component: DashboardPage },
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
  {
    path: "/corpus",
    name: "corpus",
    component: CorpusPage,
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
