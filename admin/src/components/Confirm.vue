<script setup>
// 二次确认。就两句话：标题说做什么，正文说后果；不做一整块「危险操作」。
import { useI18n } from 'vue-i18n'
import Icon from './Icon.vue'
defineProps({ title: String, text: String, action: String, busy: Boolean })
const emit = defineEmits(['ok', 'cancel'])
const { t } = useI18n()
</script>

<template>
  <div class="scrim" @click.self="emit('cancel')">
    <div class="modal" role="dialog" aria-modal="true">
      <h2>{{ title }}</h2>
      <div style="font-size:13.5px;line-height:1.6">{{ text }}</div>
      <div class="acts">
        <button type="button" class="btn outline" @click="emit('cancel')">{{ t('common.cancel') }}</button>
        <button type="button" class="btn danger" :disabled="busy" @click="emit('ok')"><Icon name="trash" :size="14" />{{ action }}</button>
      </div>
    </div>
  </div>
</template>
