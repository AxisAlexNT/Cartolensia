<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, watch } from "vue";

const props = defineProps<{
  shown: number;
  total: number;
  loading?: boolean;
  loadingAll?: boolean;
}>();

const emit = defineEmits<{
  (event: "load-more"): void;
  (event: "load-all"): void;
}>();

const sentinel = ref<HTMLElement | null>(null);
let observer: IntersectionObserver | null = null;

function canLoadMore() {
  return !props.loading && !props.loadingAll && props.shown < props.total;
}

function maybeLoadMore(entry?: IntersectionObserverEntry) {
  if ((!entry || entry.isIntersecting) && canLoadMore()) {
    emit("load-more");
  }
}

function bindObserver() {
  observer?.disconnect();
  observer = null;
  if (!sentinel.value || typeof IntersectionObserver === "undefined") return;
  observer = new IntersectionObserver(
    (entries) => {
      for (const entry of entries) maybeLoadMore(entry);
    },
    { root: null, rootMargin: "600px 0px", threshold: 0.01 }
  );
  observer.observe(sentinel.value);
}

onMounted(bindObserver);
onBeforeUnmount(() => observer?.disconnect());
watch(() => [props.shown, props.total, props.loading], () => {
  bindObserver();
});
</script>

<template>
  <div v-if="total > 0" class="paged-file-controls">
    <span>{{ shown }} of {{ total }} files shown</span>
    <button type="button" :disabled="loading || loadingAll || shown >= total" @click="emit('load-more')">
      {{ loading && !loadingAll ? "Loading..." : shown >= total ? "All files loaded" : "Load more" }}
    </button>
    <button type="button" :disabled="loading || loadingAll || shown >= total" @click="emit('load-all')">
      {{ loadingAll ? "Loading all..." : "Load all" }}
    </button>
    <small v-if="shown < total">Scroll near the bottom to load the next page automatically.</small>
    <span ref="sentinel" class="paged-file-sentinel" aria-hidden="true"></span>
  </div>
</template>
