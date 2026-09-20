<script setup>
// 消息详情。「回复」卡片是回调失败时唯一的线索：用户那边显示已回复，
// 发送方什么都没收到，两边都不会自己发现。
import { ref, onMounted, watch, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute } from 'vue-router'
import { get } from '../api'
import { onUnauthorized } from '../router'
import { setCrumbs } from '../crumbs'
import { full, hms, clock } from '../time'
import PageHead from '../components/PageHead.vue'
import Badge from '../components/Badge.vue'
import ChanChip from '../components/ChanChip.vue'
import Empty from '../components/Empty.vue'
import MessageBody from '../components/MessageBody.vue'
import ReplyReadonly from '../components/ReplyReadonly.vue'

const { t } = useI18n()
const route = useRoute()
const d = ref(null)
const err = ref('')
const m = computed(() => d.value?.message)
const now = Math.floor(Date.now() / 1000)

// multi 的答复在库里是一个 JSON 数组；直接显示会把 ["api","worker"] 摆给人看。
const answer = computed(() => {
  const raw = m.value?.reply || ''
  if (!raw.startsWith('[')) return raw
  try { return JSON.parse(raw).join(', ') } catch { return raw }
})
const hookKind = (s) => ({ 1: 'ok', 2: 'warn', 3: 'err' }[s] || 'muted')
const hookText = (s) => ({ 1: t('common.delivered'), 2: t('common.retrying'), 3: t('common.given_up') }[s] || t('common.queued'))
const pushText = (r) => (r.status === 1 ? ['ok', t('msgs.delivered_code', [r.http_status])] : r.status === 3 ? ['err', r.reason || t('common.failed')] : r.status === 2 ? ['warn', t('common.retrying')] : ['muted', t('common.queued')])

async function load() {
  err.value = ''
  try {
    d.value = await get('messages/' + route.params.uid)
    const x = m.value
    setCrumbs([{ text: t('nav.users'), to: '/admin/users' }, { text: x.owner, to: '/admin/users/' + x.user_id }, { text: x.channel_name, to: '/admin/channels/' + x.channel }, { text: (x.title || x.summary).slice(0, 24) }])
  } catch (e) { if (!onUnauthorized(e, route)) err.value = e.message }
}
onMounted(load)
watch(() => route.params.uid, load)
</script>

<template>
  <div v-if="err" class="err">{{ err }}</div>
  <template v-if="d">
    <PageHead :title="m.title || m.summary">
      <template #sub><ChanChip :id="m.channel" :name="m.channel_name" /><Badge kind="muted" :dot="false">{{ m.type }}</Badge><span>{{ full(m.created_at) }}</span></template>
    </PageHead>
    <div class="two-col" style="display:grid;grid-template-columns:minmax(0,1fr) 480px;gap:16px;align-items:start">
      <div class="col">
        <div class="card"><div class="card-h"><div class="t">{{ t('msgs.card_body') }}</div></div><div class="card-b"><MessageBody :m="m" /></div></div>
        <div class="card"><div class="card-h"><div class="t">{{ t('msgs.card_summary') }}</div></div><div class="card-b"><pre class="body">{{ m.summary }}</pre></div></div>
        <div v-if="m.extra" class="card"><div class="card-h"><div class="t">{{ t('msgs.card_extra') }}</div></div><div class="card-b"><pre class="body mono">{{ m.extra }}</pre></div></div>
      </div>
      <div v-if="m.replyable" class="card">
        <div class="card-h"><div class="t">{{ t('msgs.card_reply') }}</div></div>
        <div class="card-b" style="display:flex;flex-direction:column;gap:12px;padding-top:2px">
          <div v-if="m.replied_at" style="display:flex;align-items:center;gap:8px"><Badge kind="ok">{{ t('channel.replied') }}</Badge><b>{{ answer }}</b><span class="dim" style="font-size:12.5px">{{ full(m.replied_at) }}</span></div>
          <div v-else-if="m.reply_until > 0 && now > m.reply_until" style="display:flex;align-items:center;gap:8px"><Badge kind="warn">{{ t('msgs.reply_expired') }}</Badge><span class="dim">{{ t('msgs.reply_exp_sub', [full(m.reply_until)]) }}</span></div>
          <div v-else style="display:flex;align-items:center;gap:8px"><Badge kind="muted">{{ t('msgs.reply_waiting') }}</Badge><span class="dim">{{ m.reply_until > 0 ? t('msgs.reply_until', [full(m.reply_until)]) : t('msgs.reply_no_limit') }}</span></div>
          <ReplyReadonly :m="m" :status="false" style="border-top:0;padding-top:0" />
          <div class="dim" style="font-size:12.5px;word-break:break-all">{{ t('msgs.reply_webhook', ['']) }}<span class="mono">{{ d.reply_webhook }}</span></div>
        </div>
        <Empty v-if="m.replied_at && !d.reply_hooks.length">{{ t('msgs.reply_no_hooks') }}</Empty>
        <table v-else-if="d.reply_hooks.length">
          <thead><tr><th>{{ t('msgs.col_hook') }}</th><th>{{ t('common.attempts') }}</th><th>HTTP</th><th>{{ t('common.reason') }}</th><th>{{ t('common.time') }}</th></tr></thead>
          <tbody>
            <tr v-for="(h, i) in d.reply_hooks" :key="i">
              <td><Badge :kind="hookKind(h.status)">{{ hookText(h.status) }}</Badge></td><td class="num">{{ h.attempt }}</td>
              <td class="num dim">{{ h.status_code || t('common.dash') }}</td><td class="dim reason">{{ h.error || t('common.dash') }}</td><td class="dim num">{{ clock(h.updated_at) }}</td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
    <div class="card">
      <div class="card-h"><div class="t">{{ t('channel.delivery_log') }}</div></div>
      <Empty v-if="!d.push_log.length">{{ t('msgs.no_push_log') }}</Empty>
      <table v-else>
        <thead><tr><th>{{ t('common.device') }}</th><th>{{ t('msgs.col_env') }}</th><th>{{ t('common.status') }}</th><th>{{ t('common.attempts') }}</th><th>apns-id</th><th>{{ t('common.time') }}</th></tr></thead>
        <tbody>
          <tr v-for="(r, i) in d.push_log" :key="i">
            <td>{{ r.device }}</td><td class="dim">{{ r.apns_env }}</td>
            <td><Badge :kind="pushText(r)[0]">{{ pushText(r)[1] }}</Badge></td>
            <td class="num">{{ r.attempts }}</td><td class="mono dim">{{ r.apns_id }}</td><td class="dim num">{{ hms(r.updated_at) }}</td>
          </tr>
        </tbody>
      </table>
    </div>
  </template>
</template>
