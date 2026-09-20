<script setup>
defineProps({ kind: { type: String, default: 'muted' }, dot: { type: Boolean, default: true } })
</script>

<template>
  <span class="badge" :class="kind"><i v-if="dot" /><slot /></span>
</template>
