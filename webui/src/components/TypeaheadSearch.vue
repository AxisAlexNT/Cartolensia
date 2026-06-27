<script setup lang="ts">
import { computed, ref, watch } from "vue";

export type TypeaheadResult = {
  key: string;
  type: string;
  name: string;
  originalName?: string;
  detail?: string;
};

const props = withDefaults(defineProps<{
  modelValue: string;
  results: TypeaheadResult[];
  placeholder?: string;
  loading?: boolean;
  minLength?: number;
  goLabel?: string;
}>(), {
  placeholder: "I'm looking for...",
  loading: false,
  minLength: 2,
  goLabel: "Go to"
});

const emit = defineEmits<{
  "update:modelValue": [value: string];
  select: [item: TypeaheadResult];
  go: [];
}>();

const selectedIndex = ref(0);

const showDropdown = computed(() =>
  props.modelValue.trim().length >= props.minLength && (props.results.length > 0 || props.loading)
);

watch(() => props.results, () => {
  selectedIndex.value = 0;
});

function choose(index: number) {
  const item = props.results[index];
  if (!item) return;
  emit("select", item);
}

function onKeydown(event: KeyboardEvent) {
  if (!showDropdown.value && event.key !== "Enter") return;
  if (event.key === "ArrowDown") {
    event.preventDefault();
    selectedIndex.value = Math.min(props.results.length - 1, selectedIndex.value + 1);
  } else if (event.key === "ArrowUp") {
    event.preventDefault();
    selectedIndex.value = Math.max(0, selectedIndex.value - 1);
  } else if (event.key === "Enter") {
    event.preventDefault();
    if (props.results.length > 0) {
      choose(selectedIndex.value);
    } else {
      emit("go");
    }
  } else if (event.key === "Escape") {
    selectedIndex.value = 0;
  }
}
</script>

<template>
  <div class="typeahead-search">
    <div class="input-group input-group-sm">
      <input
        class="form-control form-control-sm"
        :value="modelValue"
        :placeholder="placeholder"
        type="search"
        @input="emit('update:modelValue', ($event.target as HTMLInputElement).value)"
        @keydown="onKeydown"
      />
      <button class="btn btn-sm btn-outline-light" type="button" @click="emit('go')">
        {{ goLabel }}
      </button>
    </div>
    <div v-if="showDropdown" class="typeahead-dropdown">
      <div v-if="loading" class="typeahead-loading">
        <span class="spinner-border spinner-border-sm me-2" role="status" aria-hidden="true"></span>
        Searching...
      </div>
      <button
        v-for="(item, idx) in results"
        :key="item.key"
        type="button"
        class="typeahead-result"
        :class="{ active: idx === selectedIndex }"
        @click="choose(idx)"
      >
        <span class="typeahead-type">{{ item.type }}</span>
        <span class="typeahead-name">{{ item.name }}</span>
        <span v-if="item.originalName" class="typeahead-original">({{ item.originalName }})</span>
        <small v-if="item.detail" class="typeahead-detail">{{ item.detail }}</small>
      </button>
    </div>
  </div>
</template>
