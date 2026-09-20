<script setup>
import { useRoute } from 'vue-router'
import Shell from './components/Shell.vue'
const route = useRoute()
</script>

<template>
  <router-view v-if="route.name === 'login'" />
  <Shell v-else>
    <router-view />
  </Shell>
</template>
