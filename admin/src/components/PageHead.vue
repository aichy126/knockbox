<script setup>
defineProps({ title: String })
</script>

<template>
  <div class="ph">
    <div>
      <h1>{{ title }}</h1>
      <div v-if="$slots.sub" class="sub"><slot name="sub" /></div>
    </div>
    <div class="spacer" />
    <div v-if="$slots.acts" class="acts"><slot name="acts" /></div>
  </div>
</template>
