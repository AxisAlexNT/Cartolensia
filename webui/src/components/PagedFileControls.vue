<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from "vue";

const props = defineProps<{
  shown: number;
  total: number;
  loading?: boolean;
  loadingAll?: boolean;
  label?: string;
}>();

const emit = defineEmits<{
  (event: "load-more"): void;
  (event: "load-all"): void;
}>();

const sentinel = ref<HTMLElement | null>(null);
let observer: IntersectionObserver | null = null;
let scrollTicking = false;

const itemLabel = computed(() => props.label || "files");

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

function handleWindowScroll() {
  if (scrollTicking) return;
  scrollTicking = true;
  window.requestAnimationFrame(() => {
    scrollTicking = false;
    if (!canLoadMore()) return;
    const doc = document.documentElement;
    const scrollBottom = window.scrollY + window.innerHeight;
    const pageBottom = Math.max(doc.scrollHeight, document.body.scrollHeight);
    if (pageBottom - scrollBottom < 900) {
      emit("load-more");
    }
  });
}

onMounted(() => {
  bindObserver();
  window.addEventListener("scroll", handleWindowScroll, { passive: true });
  handleWindowScroll();
});
onBeforeUnmount(() => {
  observer?.disconnect();
  window.removeEventListener("scroll", handleWindowScroll);
});
watch(() => [props.shown, props.total, props.loading, props.loadingAll], () => {
  bindObserver();
  handleWindowScroll();
});
</script>

<template>
  <div v-if="total > 0" class="paged-file-controls">
    <span>{{ shown }} of {{ total }} {{ itemLabel }} shown</span>
    <button type="button" class="btn btn-sm btn-outline-primary" :disabled="loading || loadingAll || shown >= total" @click="emit('load-more')">
      <span v-if="loading && !loadingAll" class="spinner-border spinner-border-sm" aria-hidden="true"></span>
      {{ loading && !loadingAll ? "Loading..." : shown >= total ? `All ${itemLabel} loaded` : "Load more" }}
    </button>
    <button type="button" class="btn btn-sm btn-outline-secondary" :disabled="loading || loadingAll || shown >= total" @click="emit('load-all')">
      <span v-if="loadingAll" class="spinner-border spinner-border-sm" aria-hidden="true"></span>
      {{ loadingAll ? "Loading all..." : "Load all" }}
    </button>
    <small v-if="shown < total">Scroll near the bottom to load the next page automatically.</small>
    <small v-if="loading || loadingAll" class="paged-loading-note">
      <span class="spinner-border spinner-border-sm" aria-hidden="true"></span>
      Loading more {{ itemLabel }}...
    </small>
    <span ref="sentinel" class="paged-file-sentinel" aria-hidden="true"></span>
  </div>
</template>
