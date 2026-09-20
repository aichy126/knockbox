<script setup>
import { useI18n } from 'vue-i18n'
import Badge from './Badge.vue'
const props = defineProps({ ok: { type: Number, default: 0 }, total: { type: Number, default: 0 } })
const { t } = useI18n()
const kind = () => (props.total === 0 ? 'muted' : props.ok === props.total ? 'ok' : props.ok === 0 ? 'err' : 'warn')
</script>

<template>
  <Badge :kind="kind()"><template v-if="total === 0">{{ t('common.not_pushed') }}</template><template v-else>{{ ok }}/{{ total }}</template></Badge>
</template>
