import { createRouter, createMemoryHistory } from "vue-router";
import DashboardView from "../views/DashboardView.vue";
import DomainView from "../views/DomainView.vue";
import ConceptView from "../views/ConceptView.vue";
import FlashcardSession from "../views/FlashcardSession.vue";
import FeynmanModule from "../views/FeynmanModule.vue";

const routes = [
  {
    path: "/",
    name: "dashboard",
    component: DashboardView,
  },
  {
    path: "/domain/:domainId",
    name: "domain",
    component: DomainView,
    props: true,
  },
  {
    path: "/domain/:domainId/concept/:conceptId",
    name: "concept",
    component: ConceptView,
    props: true,
  },
  {
    path: "/domain/:domainId/concept/:conceptId/flashcards",
    name: "flashcards",
    component: FlashcardSession,
    props: true,
  },
  {
    path: "/domain/:domainId/concept/:conceptId/feynman",
    name: "feynman",
    component: FeynmanModule,
    props: true,
  },
];

export const router = createRouter({
  history: createMemoryHistory(),
  routes,
});
