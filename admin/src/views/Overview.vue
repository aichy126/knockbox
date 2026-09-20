<script setup>
import { ref, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute } from 'vue-router'
import { get } from '../api'
import { me } from '../store'
import { onUnauthorized } from '../router'
import { setCrumbs } from '../crumbs'
import { clock } from '../time'
import PageHead from '../components/PageHead.vue'
import Badge from '../components/Badge.vue'
import PushBadge from '../components/PushBadge.vue'
import ChanChip from '../components/ChanChip.vue'
import Empty from '../components/Empty.vue'
import Icon from '../components/Icon.vue'

const { t } = useI18n()
const route = useRoute()
const d = ref(null)
const err = ref('')

const rate = () => (d.value.push.rate == null ? t('common.dash') : d.value.push.rate.toFixed(1) + '%')
const rateKind = () => { const r = d.value.push.rate; return r == null ? 'muted' : r >= 99 ? 'ok' : r >= 90 ? 'warn' : 'err' }

onMounted(async () => {
  setCrumbs([{ text: t('nav.dash') }])
  try { d.value = await get('overview') } catch (e) { if (!onUnauthorized(e, route)) err.value = e.message }
})
</script>

<template>
  <PageHead :title="t('dash.title')"><template #sub>{{ me?.server.name }}{{ t('dash.last24h') }}</template></PageHead>
  <div v-if="err" class="err">{{ err }}</div>
  <template v-if="d">
    <div class="grid4">
      <div class="card stat"><div class="l">{{ t('dash.msgs24h') }}</div><div class="v">{{ d.messages.current }}</div><div class="b"><Badge kind="muted">{{ t('dash.prev_day', [d.messages.prev]) }}</Badge></div></div>
      <div class="card stat"><div class="l">{{ t('dash.push_rate') }}</div><div class="v">{{ rate() }}</div><div class="b"><Badge :kind="rateKind()">{{ t('dash.fail_retry', [d.push.failed, d.push.retrying]) }}</Badge></div></div>
      <div class="card stat"><div class="l">{{ t('dash.reachable') }}</div><div class="v">{{ d.devices.pushable }}</div><div class="b"><Badge kind="info">{{ t('dash.sandbox_count', [d.devices.sandbox]) }}</Badge></div></div>
      <div class="card stat"><div class="l">{{ t('dash.channels') }}</div><div class="v">{{ d.channels.total }}</div><div class="b"><Badge kind="muted">{{ t('dash.muted_count', [d.channels.muted]) }}</Badge></div></div>
    </div>
    <div class="two-col" style="display:grid;grid-template-columns:2fr 1fr;gap:16px;align-items:start">
      <div class="card">
        <div class="card-h"><div class="t">{{ t('dash.recent') }}</div></div>
        <Empty v-if="!d.recent.length">{{ t('dash.empty') }}</Empty>
        <table v-else>
          <thead><tr><th>{{ t('common.time') }}</th><th>{{ t('common.channel') }}</th><th>{{ t('common.type') }}</th><th>{{ t('common.title') }}</th><th>{{ t('common.push') }}</th></tr></thead>
          <tbody>
            <tr v-for="m in d.recent" :key="m.uid">
              <td class="dim num">{{ clock(m.created_at) }}</td>
              <td><ChanChip :id="m.channel" :name="m.channel_name" /></td>
              <td><Badge kind="muted" :dot="false">{{ m.type }}</Badge></td>
              <td style="font-weight:500;white-space:normal"><router-link :to="'/admin/messages/' + m.uid">{{ m.title || m.summary }}</router-link></td>
              <td><PushBadge :ok="m.push.ok" :total="m.push.total" /></td>
            </tr>
          </tbody>
        </table>
      </div>
      <div class="col">
        <div class="card">
          <div class="card-h"><div class="t">{{ t('dash.failures24h') }}</div></div>
          <div class="card-b" style="display:flex;flex-direction:column;gap:10px">
            <Badge v-if="!d.failures.length" kind="ok">{{ t('dash.no_failures') }}</Badge>
            <div v-for="f in d.failures" :key="f.reason + f.http_status" style="display:flex;align-items:center;gap:10px">
              <span class="mono">{{ f.reason || t('common.no_reason') }}</span><Badge v-if="f.http_status" kind="muted" :dot="false">{{ f.http_status }}</Badge><div class="spacer" /><span class="num" style="font-weight:600">{{ f.count }}</span>
            </div>
            <div v-if="d.failures.length" class="note"><Icon name="alert" :size="14" /><div v-html="t('dash.unreg_note')" /></div>
          </div>
        </div>
        <div v-if="me" class="card">
          <div class="card-h"><div class="t">{{ t('settings.card_server') }}</div></div>
          <div class="card-b kv">
            <div class="k">{{ t('settings.info_name') }}</div><div class="v">{{ me.server.name }}</div>
            <div class="k">{{ t('settings.info_url') }}</div><div class="v mono">{{ me.server.external_url }}</div>
            <div class="k">{{ t('settings.info_version') }}</div><div class="v mono">{{ me.server.version }}</div>
            <div class="k">{{ t('settings.info_mode') }}</div><div class="v"><Badge :kind="me.server.public_enabled ? 'info' : 'muted'" :dot="false">{{ me.server.public_enabled ? t('settings.mode_public') : t('settings.mode_self') }}</Badge></div>
          </div>
        </div>
      </div>
    </div>
  </template>
</template>
