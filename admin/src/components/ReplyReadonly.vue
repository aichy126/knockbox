<script setup>
// 可回复消息的回复区，只读。发了什么就画什么：单选画按钮、多选画勾选框、
// 数值画滑块、文本画输入框，和 app 里那一屏是同一套。
// 一律不可点、不发任何请求：后台是主人查看的地方，不是替用户作答的地方。
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Badge from './Badge.vue'
import { full, hms } from '../time'

const props = defineProps({ m: { type: Object, required: true }, status: { type: Boolean, default: true } })
const { t } = useI18n()

const spec = computed(() => { try { return JSON.parse(props.m.extra || '{}').reply || null } catch { return null } })
const replied = computed(() => !!props.m.replied_at)
const now = Math.floor(Date.now() / 1000)
const expired = computed(() => !replied.value && props.m.reply_until > 0 && now > props.m.reply_until)
const picked = computed(() => {
  if (!replied.value) return []
  if (spec.value?.type === 'multi') { try { return JSON.parse(props.m.reply) } catch { return [] } }
  return [props.m.reply]
})
const trim = (v) => String(Number(v))
const range = computed(() => {
  const s = spec.value || {}
  const lo = s.min ?? 0, hi = s.max ?? 100, st = s.step > 0 ? s.step : 1
  const val = replied.value ? Number(props.m.reply) : lo + (hi - lo) / 2
  return { lo, hi, st, val, unit: s.unit || '' }
})
</script>

<template>
  <div v-if="m.replyable && spec" class="reply">
    <div v-if="spec.type === 'choice'" class="choice">
      <span v-for="o in spec.options" :key="o" :class="{ on: picked.includes(o) }">{{ o }}</span>
    </div>
    <div v-else-if="spec.type === 'multi'" class="checks">
      <label v-for="o in spec.options" :key="o" :class="picked.includes(o) ? 'on' : replied ? 'off' : ''">
        <input type="checkbox" disabled :checked="picked.includes(o)"><span>{{ o }}</span>
      </label>
    </div>
    <div v-else-if="spec.type === 'number'" class="slider">
      <span class="big" :class="{ faint: !replied }">{{ trim(range.val) }}<small class="dim"> {{ range.unit }}</small></span>
      <input type="range" disabled :min="range.lo" :max="range.hi" :step="range.st" :value="range.val">
      <div class="ends"><span>{{ trim(range.lo) }}{{ range.unit }}</span><span>{{ trim(range.hi) }}{{ range.unit }}</span></div>
    </div>
    <div v-else-if="spec.type === 'text'">
      <input type="text" disabled class="rtext" :class="{ filled: replied }" :value="replied ? m.reply : ''" :placeholder="t('channel.waiting_text')">
    </div>
    <div v-else class="dim" style="font-size:12px">{{ t('channel.unknown_reply') }}{{ spec.type }}</div>

    <div v-if="status" class="rstatus">
      <template v-if="replied"><Badge kind="ok">{{ t('channel.replied') }}</Badge><span>{{ full(m.replied_at) }}</span></template>
      <template v-else-if="expired"><span style="color:var(--warning)">{{ t('channel.expired', [hms(m.reply_until)]) }}</span></template>
      <template v-else-if="m.reply_until > 0"><span>{{ t('channel.waiting_till', [hms(m.reply_until)]) }}</span></template>
      <template v-else><span>{{ t('channel.waiting_open') }}</span></template>
    </div>
  </div>
</template>
