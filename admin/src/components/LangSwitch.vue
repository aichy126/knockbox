<script setup>
import { useI18n } from 'vue-i18n'
import { setLang, otherLang, langName } from '../i18n'
import Icon from './Icon.vue'
defineProps({ fixed: Boolean })
const { locale } = useI18n()
const other = () => otherLang()
</script>

<template>
  <button type="button" class="langsw" :class="{ fixed }" :lang="other()" @click="setLang(other())">
    <Icon name="globe" :size="14" />{{ langName(other()) }}
  </button>
</template>
