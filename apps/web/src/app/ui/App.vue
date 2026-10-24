<template>
  <div class="study-dashboard">
    <header class="study-dashboard__header">
      <button
        class="study-dashboard__back"
        v-if="showBack"
        @click="goBack"
        :aria-label="'Volver'"
      >
        ← Volver
      </button>
      <h1 class="study-dashboard__title">Estudio</h1>
      <!-- Two destinations, both top level: the launcher and the topic list.
           The back button is for anything deeper where the trail is not obvious. -->
      <nav class="study-dashboard__nav" aria-label="Secciones">
        <RouterLink
          class="study-dashboard__link"
          :class="{ 'study-dashboard__link--active': route.name === 'dashboard' }"
          to="/"
        >
          Recursos
        </RouterLink>
        <RouterLink
          class="study-dashboard__link"
          :class="{ 'study-dashboard__link--active': route.name === 'topics' }"
          to="/topics"
        >
          Temas
        </RouterLink>
      </nav>
    </header>
    <main class="study-dashboard__main">
      <router-view />
    </main>
  </div>
</template>

<script setup lang="ts">
import { computed } from "vue";
import { useRouter, useRoute } from "vue-router";

const router = useRouter();
const route = useRoute();

const showBack = computed(
  () => route.name !== "dashboard" && route.name !== "topics",
)

function goBack() {
  router.back();
}
</script>

<style scoped>
.study-dashboard {
  display: flex;
  flex-direction: column;
  height: 100%;
  color: var(--text-normal);
  font-family: var(--font-text, inherit);
}

.study-dashboard__header {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  padding: 0.5rem 0.75rem;
  border-bottom: 1px solid var(--background-modifier-border);
  min-height: 44px;
}

.study-dashboard__back {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 44px;
  min-height: 44px;
  padding: 0.25rem 0.5rem;
  border: 1px solid var(--background-modifier-border);
  border-radius: 6px;
  background: var(--background-secondary);
  color: var(--text-normal);
  cursor: pointer;
  font-size: 0.85rem;
  transition: background-color 0.15s ease;
}

.study-dashboard__back:hover {
  background: var(--background-modifier-hover);
}

.study-dashboard__title {
  margin: 0;
  font-size: 1rem;
  font-weight: 600;
  line-height: 44px;
}

.study-dashboard__nav {
  display: flex;
  gap: 0.25rem;
  margin-left: auto;
}

.study-dashboard__link {
  display: inline-flex;
  align-items: center;
  min-height: 44px;
  padding: 0 0.625rem;
  border-radius: 6px;
  color: var(--text-muted);
  font-size: 0.85rem;
  text-decoration: none;
  transition: color 0.15s ease, background-color 0.15s ease;
}

.study-dashboard__link:hover {
  background: var(--background-modifier-hover);
  color: var(--text-normal);
}

.study-dashboard__link--active {
  color: var(--text-accent);
  font-weight: 600;
}

.study-dashboard__main {
  flex: 1;
  overflow-y: auto;
  padding: 0.75rem;
}
</style>
