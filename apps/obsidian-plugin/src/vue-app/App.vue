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
      <h1 class="study-dashboard__title">Panel de Estudio</h1>
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

const showBack = computed(() => route.path !== "/");

function goBack() {
  const path = route.path;

  // /topic/:topicId/concept/:conceptId/* → /topic/:topicId
  const conceptMatch = path.match(/^(\/topic\/[^/]+)\/concept\/[^/]+/);
  if (conceptMatch) {
    router.push(conceptMatch[1]);
    return;
  }

  // /topic/:topicId → /
  if (path.startsWith("/topic/")) {
    router.push({ name: "dashboard" });
    return;
  }

  // /study/* → /
  router.push({ name: "dashboard" });
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

.study-dashboard__main {
  flex: 1;
  overflow-y: auto;
  padding: 0.75rem;
}
</style>
