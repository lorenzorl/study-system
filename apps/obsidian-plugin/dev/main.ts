import { createApp } from "vue";
import { createRouter, createWebHistory } from "vue-router";
import { createPinia } from "pinia";
import App from "../src/vue-app/App.vue";
import DashboardView from "../src/vue-app/views/DashboardView.vue";
import DomainView from "../src/vue-app/views/DomainView.vue";
import ConceptView from "../src/vue-app/views/ConceptView.vue";
import FlashcardSession from "../src/vue-app/views/FlashcardSession.vue";
import FeynmanModule from "../src/vue-app/views/FeynmanModule.vue";

const routes = [
  { path: "/", name: "dashboard", component: DashboardView },
  { path: "/domain/:domainId", name: "domain", component: DomainView, props: true },
  { path: "/domain/:domainId/concept/:conceptId", name: "concept", component: ConceptView, props: true },
  { path: "/domain/:domainId/concept/:conceptId/flashcards", name: "flashcards", component: FlashcardSession, props: true },
  { path: "/domain/:domainId/concept/:conceptId/feynman", name: "feynman", component: FeynmanModule, props: true },
];

const router = createRouter({
  history: createWebHistory(),
  routes,
});

const app = createApp(App);
app.use(createPinia());
app.use(router);
app.mount("#app");
