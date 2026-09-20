<script setup>
import { ref, onMounted, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute } from 'vue-router'
import { get, post } from '../api'
import { onUnauthorized } from '../router'
import { setCrumbs } from '../crumbs'
import { ago, clock, ymd, hm } from '../time'
import PageHead from '../components/PageHead.vue'
import Badge from '../components/Badge.vue'
import ChanChip from '../components/ChanChip.vue'
import Empty from '../components/Empty.vue'
import Confirm from '../components/Confirm.vue'
import Icon from '../components/Icon.vue'

const { t } = useI18n()
const route = useRoute()
const d = ref(null)
const err = ref('')
const revoking = ref(null)
const busy = ref(false)

const lim = (n, max) => (max > 0 ? `${n} / ${max}` : String(n))
const now = Math.floor(Date.now() / 1000)
const state = (c) => (c.muted ? ['muted', t('mute.always')] : c.mute_until > now ? ['warn', t('mute.until', [hm(c.mute_until)])] : ['ok', t('member.state_ok')])

async function load() {
  err.value = ''
  try {
    d.value = await get('members/' + route.params.id)
    setCrumbs([{ text: t('nav.users'), to: '/admin/users' }, { text: d.value.member.name }])
  } catch (e) { if (!onUnauthorized(e, route)) err.value = e.message }
}
async function toggle() {
  try { const r = await post(`members/${d.value.member.id}/unlimited`, { on: !d.value.member.unlimited }); d.value.member.unlimited = r.unlimited }
  catch (e) { if (!onUnauthorized(e, route)) err.value = e.message }
}
async function revoke() {
  busy.value = true
  try { await post(`devices/${revoking.value.id}/revoke`); revoking.value = null; await load() }
  catch (e) { if (!onUnauthorized(e, route)) err.value = e.message }
  finally { busy.value = false }
}

onMounted(load)
watch(() => route.params.id, load)
</script>

<template>
  <div v-if="err" class="err">{{ err }}</div>
  <template v-if="d">
    <PageHead :title="d.member.name">
      <template #sub>{{ t('member.created_on', [ymd(d.member.created_at)]) }}</template>
      <template #acts>
        <Badge :kind="d.member.role === 'admin' ? 'info' : 'muted'" :dot="false">{{ d.member.role === 'admin' ? t('common.role_admin') : t('common.role_member') }}</Badge>
        <button type="button" class="btn sm" :class="d.member.unlimited ? 'outline' : 'ghost'" @click="toggle">{{ d.member.unlimited ? t('users.unlimited_on') : t('users.unlimited') }}</button>
      </template>
    </PageHead>
    <div class="grid4">
      <div class="card stat"><div class="l">{{ t('member.stat_channels') }}</div><div class="v">{{ d.usage.channels }}<small v-if="d.usage.channel_limit > 0"> / {{ d.usage.channel_limit }}</small></div><div class="sub">{{ t('member.stat_now') }}</div></div>
      <div class="card stat"><div class="l">{{ t('member.stat_msgs24h') }}</div><div class="v">{{ d.usage.today }}<small v-if="d.usage.daily_limit > 0"> / {{ d.usage.daily_limit }}</small></div><div class="sub">{{ t('member.stat_rolling') }}</div></div>
      <div class="card stat"><div class="l">{{ t('member.stat_total') }}</div><div class="v">{{ d.usage.messages }}</div><div class="sub">{{ t('member.stat_kept') }}</div></div>
      <div class="card stat"><div class="l">{{ t('member.stat_files') }}</div><div class="v">{{ (d.usage.file_bytes / 1e6).toFixed(1) }} MB</div><div class="sub">{{ t('member.stat_derived') }}</div></div>
    </div>
    <div class="card">
      <div class="card-h"><div class="t">{{ t('member.card_channels') }}</div></div>
      <Empty v-if="!d.channels.length">{{ t('member.no_channels') }}</Empty>
      <table v-else>
        <thead><tr><th>{{ t('common.name') }}</th><th>{{ t('common.messages') }}</th><th>{{ t('member.col_last') }}</th><th>{{ t('member.col_sound') }}</th><th>{{ t('member.col_level') }}</th><th>{{ t('common.status') }}</th></tr></thead>
        <tbody>
          <tr v-for="c in d.channels" :key="c.id">
            <td><ChanChip :id="c.id" :name="c.name" :meta="c.meta" /></td>
            <td class="num">{{ c.messages }}</td><td class="dim num">{{ clock(c.last_msg_at) }}</td>
            <td class="dim mono">{{ c.sound }}</td><td class="dim mono">{{ c.level }}</td>
            <td><Badge :kind="state(c)[0]">{{ state(c)[1] }}</Badge></td>
          </tr>
        </tbody>
      </table>
    </div>
    <div class="card">
      <div class="card-h"><div class="t">{{ t('member.card_devices') }}</div><div class="spacer" /><router-link to="/admin/pair" class="btn ghost sm"><Icon name="qr" :size="14" />{{ t('member.pair_new') }}</router-link></div>
      <Empty v-if="!d.devices.length">{{ t('member.no_devices') }}</Empty>
      <table v-else>
        <thead><tr><th>{{ t('common.device') }}</th><th>{{ t('member.col_os') }}</th><th>{{ t('member.col_app') }}</th><th>APNs</th><th>{{ t('member.col_synced') }}</th><th>{{ t('member.col_seen') }}</th><th></th></tr></thead>
        <tbody>
          <tr v-for="v in d.devices" :key="v.id">
            <td style="font-weight:500">{{ v.name }}<div class="dim" style="font-weight:400;font-size:12px">{{ v.model }}</div></td>
            <td class="dim">{{ v.platform }} {{ v.os_version }}</td><td class="dim mono">{{ v.app_version }}</td>
            <td><Badge v-if="!v.can_push" kind="err">{{ t('member.no_push') }}</Badge><Badge v-else :kind="v.apns_env === 'sandbox' ? 'muted' : 'info'" :dot="false">{{ v.apns_env }}</Badge></td>
            <td class="dim mono num">{{ v.sync_rev }}</td><td class="dim">{{ ago(v.last_seen_at, t) }}</td>
            <td class="r"><button type="button" class="btn ghost sm" @click="revoking = v">{{ t('member.revoke') }}</button></td>
          </tr>
        </tbody>
      </table>
    </div>
  </template>
  <Confirm v-if="revoking" :title="t('member.revoke')" :text="t('member.revoke_ask')" :action="t('member.revoke')" :busy="busy" @ok="revoke" @cancel="revoking = null" />
</template>
