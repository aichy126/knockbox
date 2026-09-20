<script setup>
import { icons } from '../icons'
defineProps({ name: { type: String, required: true }, size: { type: Number, default: 16 } })
</script>

<template>
  <svg :width="size" :height="size" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true" v-html="icons[name]" />
</template>
