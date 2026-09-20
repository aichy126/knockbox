<script setup>
// 一条消息的正文。markdown 才渲染，纯文本原样保留换行——
// 不能把一段 shell 输出当成 markdown 去解释。
import { computed } from 'vue'
import { marked } from 'marked'
import DOMPurify from 'dompurify'
import Badge from './Badge.vue'

const props = defineProps({ m: { type: Object, required: true }, q: { type: String, default: '' } })

const esc = (s) => String(s).replace(/[&<>"]/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;' }[c]))
// 命中高亮只做在已经转义过的文本上，所以不会把用户内容当标记。
const hl = (html) => {
  if (!props.q) return html
  const re = new RegExp(props.q.replace(/[.*+?^${}()|[\]\\]/g, '\\$&'), 'ig')
  return html.replace(/>([^<]+)</g, (m, text) => '>' + text.replace(re, (x) => `<mark class="hl">${x}</mark>`) + '<')
}
const items = computed(() => { try { return JSON.parse(props.m.extra || '{}').items || [] } catch { return [] } })
const kind = (s) => ({ ok: 'ok', warn: 'warn', error: 'err' }[s] || 'muted')
const html = computed(() => {
  const m = props.m
  const txt = m.body || m.summary || ''
  if (m.type === 'markdown') return hl(DOMPurify.sanitize(marked.parse(txt)))
  return hl(`<p>${esc(txt).replace(/\n/g, '<br>')}</p>`)
})
</script>

<template>
  <div v-if="m.type === 'card' && items.length" style="display:flex;flex-wrap:wrap;gap:6px">
    <Badge v-for="(it, i) in items" :key="i" :kind="kind(it.style)">{{ it.k }} <b style="margin-left:4px">{{ it.v }}</b></Badge>
  </div>
  <div v-else-if="m.type === 'image' || m.type === 'file'" class="body">
    <div v-if="m.summary" class="dim" style="margin-bottom:8px">{{ m.summary }}</div>
    <img v-if="m.file_url" :src="m.file_url" alt="">
  </div>
  <div v-else class="body" v-html="html" />
</template>
