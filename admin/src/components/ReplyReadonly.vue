<script setup>
// 可回复消息的回复区，只读。发了什么就画什么：单选画按钮、多选画勾选框、
// 数值画滑块、文本画输入框，和 app 里那一屏是同一套。
// 组合回复（type=form）逐个控件画，每个控件上方是它的 label。
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
const isForm = computed(() => spec.value?.type === 'form')

// 库里的答复：单控件是一个值（multi 是 JSON 数组的字符串），组合回复是 {控件 id: 答案}。
// 选填控件空着交时，它的 id 不在对象里。
function singleValue(s, raw) {
  if (s.type === 'multi') { try { return JSON.parse(raw) } catch { return [] } }
  if (s.type === 'number') return Number(raw)
  return raw
}
const answers = computed(() => {
  if (!replied.value || !isForm.value) return {}
  try { return JSON.parse(props.m.reply) || {} } catch { return {} }
})

const trim = (v) => String(Number(v))
function control(key, label, s, value) {
  const answered = value !== undefined && value !== null && value !== ''
  const lo = s.min ?? 0, hi = s.max ?? 100, st = s.step > 0 ? s.step : 1
  return {
    key, label, s, answered,
    picked: !answered ? [] : Array.isArray(value) ? value : [value],
    text: answered ? String(value) : '',
    range: { lo, hi, st, val: answered ? Number(value) : lo + (hi - lo) / 2, unit: s.unit || '' },
  }
}
const controls = computed(() => {
  const s = spec.value
  if (!s) return []
  if (isForm.value) return (s.fields || []).map((f) => control(f.id, f.label || '', f, answers.value[f.id]))
  return [control('reply', '', s, replied.value ? singleValue(s, props.m.reply) : undefined)]
})
</script>

<template>
  <div v-if="m.replyable && spec" class="reply">
    <div v-for="c in controls" :key="c.key" class="rfield">
      <div v-if="c.label || (replied && !c.answered && isForm)" class="rlabel">
        <span>{{ c.label }}</span><span v-if="replied && !c.answered" class="dim">{{ t('channel.left_empty') }}</span>
      </div>
      <div v-if="c.s.type === 'choice'" class="choice">
        <span v-for="o in c.s.options" :key="o" :class="{ on: c.picked.includes(o) }">{{ o }}</span>
      </div>
      <div v-else-if="c.s.type === 'multi'" class="checks">
        <label v-for="o in c.s.options" :key="o" :class="c.picked.includes(o) ? 'on' : replied ? 'off' : ''">
          <input type="checkbox" disabled :checked="c.picked.includes(o)"><span>{{ o }}</span>
        </label>
      </div>
      <div v-else-if="c.s.type === 'number'" class="slider">
        <span class="big" :class="{ faint: !c.answered }">{{ trim(c.range.val) }}<small class="dim"> {{ c.range.unit }}</small></span>
        <input type="range" disabled :min="c.range.lo" :max="c.range.hi" :step="c.range.st" :value="c.range.val">
        <div class="ends"><span>{{ trim(c.range.lo) }}{{ c.range.unit }}</span><span>{{ trim(c.range.hi) }}{{ c.range.unit }}</span></div>
      </div>
      <div v-else-if="c.s.type === 'text'">
        <input type="text" disabled class="rtext" :class="{ filled: c.answered }" :value="c.text" :placeholder="replied ? '' : c.s.placeholder || t('channel.waiting_text')">
      </div>
      <div v-else class="dim" style="font-size:12px">{{ t('channel.unknown_reply') }}{{ c.s.type }}</div>
    </div>
    <div v-if="isForm && !controls.length" class="dim" style="font-size:12px">{{ t('channel.unknown_reply') }}form</div>

    <div v-if="status" class="rstatus">
      <template v-if="replied"><Badge kind="ok">{{ t('channel.replied') }}</Badge><span>{{ full(m.replied_at) }}</span></template>
      <template v-else-if="expired"><span style="color:var(--warning)">{{ t('channel.expired', [hms(m.reply_until)]) }}</span></template>
      <template v-else-if="m.reply_until > 0"><span>{{ t('channel.waiting_till', [hms(m.reply_until)]) }}</span></template>
      <template v-else><span>{{ t('channel.waiting_open') }}</span></template>
    </div>
  </div>
</template>
