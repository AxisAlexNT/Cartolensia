<script setup lang="ts">
type MonthBucket = {
  month: string;
  count: number;
};

const props = defineProps<{
  buckets: MonthBucket[];
  modelValue: string;
}>();

const emit = defineEmits<{
  (event: "update:modelValue", value: string): void;
}>();

function select(value: string) {
  emit("update:modelValue", value);
}
</script>

<template>
  <section class="month-filter-bar" aria-label="Date filter">
    <label>
      Month
      <select :value="props.modelValue" @change="select(($event.target as HTMLSelectElement).value)">
        <option value="">All months</option>
        <option v-for="bucket in props.buckets" :key="bucket.month" :value="bucket.month">
          {{ bucket.month }} · {{ bucket.count }}
        </option>
      </select>
    </label>
    <details v-if="props.buckets.length > 0" class="month-filter-details">
      <summary>Quick months</summary>
      <div class="month-filter-buttons">
        <button type="button" :class="{ active: props.modelValue === '' }" @click="select('')">All</button>
        <button
          v-for="bucket in props.buckets.slice(0, 18)"
          :key="bucket.month"
          type="button"
          :class="{ active: props.modelValue === bucket.month }"
          @click="select(bucket.month)"
        >
          {{ bucket.month }} · {{ bucket.count }}
        </button>
      </div>
    </details>
  </section>
</template>
