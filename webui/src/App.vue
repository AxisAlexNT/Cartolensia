<script setup lang="ts">
const sections = [
  { name: "Explorer", status: "planned" },
  { name: "Jobs", status: "planned" },
  { name: "Settings", status: "planned" }
];
</script>

<template>
  <main class="shell">
    <header class="topbar">
      <h1>Cartolensia</h1>
      <span>Fixture-first MVP scaffold</span>
    </header>

    <section class="workspace" aria-label="MVP sections">
      <article v-for="section in sections" :key="section.name" class="panel">
        <h2>{{ section.name }}</h2>
        <p>{{ section.status }}</p>
      </article>
    </section>
  </main>
</template>
