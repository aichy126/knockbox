<script setup>
// 频道详情：消息流，正序、停在最新那条，和 app 一致。
// 搜索就在这里——消息属于频道，找一条消息就是在它的频道里找。
import { ref, onMounted, watch, nextTick } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import { get, post, qs } from '../api'
import { onUnauthorized } from '../router'
import { setCrumbs } from '../crumbs'
import { plural } from '../i18n'
import { ago, hm, dayLabel, ymd } from '../time'
import PageHead from '../components/PageHead.vue'
import Badge from '../components/Badge.vue'
import PushBadge from '../components/PushBadge.vue'
import Empty from '../components/Empty.vue'
import Confirm from '../components/Confirm.vue'
import Icon from '../components/Icon.vue'
import MessageBody from '../components/MessageBody.vue'
import ReplyReadonly from '../components/ReplyReadonly.vue'

const { t, locale } = useI18n()
const route = useRoute()
const router = useRouter()
const d = ref(null)
const items = ref([])
const hasMore = ref(false)
const cursor = ref('')
const q = ref(route.query.q || '')
const applied = ref(q.value)
const err = ref('')
const purging = ref(false)
const busy = ref(false)
const stream = ref(null)

const id = () => route.params.id
const now = Math.floor(Date.now() / 1000)
const muteText = () => (d.value.channel.muted ? t('mute.always') : d.value.channel.mute_until > now ? t('mute.until', [hm(d.value.channel.mute_until)]) : '')

async function loadHead() {
  d.value = await get('channels/' + id())
  setCrumbs([{ text: t('nav.users'), to: '/admin/users' }, { text: d.value.owner.name, to: '/admin/users/' + d.value.owner.id }, { text: d.value.channel.name }])
}

// 不带关键词是流：倒序查最近 N 条、正序显示。带关键词是搜索：命中的按新到旧列。
async function loadMessages(more = false) {
  const c = more ? cursor.value : ''
  if (applied.value) {
    const r = await get('messages' + qs({ channel: id(), q: applied.value, cursor: c }))
    items.value = more ? items.value.concat(r.items) : r.items
    hasMore.value = r.has_more; cursor.value = r.next_cursor || ''
  } else {
    const r = await get(`channels/${id()}/messages` + qs({ cursor: c }))
    items.value = more ? r.items.concat(items.value) : r.items
    hasMore.value = r.has_more; cursor.value = r.next_cursor || ''
    if (!more) { await nextTick(); stream.value?.lastElementChild?.scrollIntoView({ block: 'end' }) }
  }
}

async function load() {
  err.value = ''
  try { await loadHead(); await loadMessages() }
  catch (e) { if (!onUnauthorized(e, route)) err.value = e.message }
}
function search() {
  applied.value = q.value.trim()
  router.replace({ query: applied.value ? { q: applied.value } : {} })
  loadMessages().catch((e) => { if (!onUnauthorized(e, route)) err.value = e.message })
}
function clear() { q.value = ''; search() }
async function purge() {
  busy.value = true
  try { await post(`channels/${id()}/purge`); purging.value = false; await load() }
  catch (e) { if (!onUnauthorized(e, route)) err.value = e.message }
  finally { busy.value = false }
}
// 同一天的消息共用一个日期分隔。
const dayOf = (i) => { const cur = ymd(items.value[i].created_at); return i === 0 || ymd(items.value[i - 1].created_at) !== cur ? dayLabel(items.value[i].created_at, t, locale.value) : '' }

onMounted(load)
watch(() => route.params.id, load)
</script>

<template>
  <div v-if="err" class="err">{{ err }}</div>
  <template v-if="d">
    <PageHead :title="d.channel.name">
      <template #sub>{{ t('channel.belongs_to', [d.owner.name]) }} · {{ d.channel.sound }} · {{ d.channel.level }}<template v-if="muteText()"> · <span style="color:var(--warning)">{{ muteText() }}</span></template></template>
      <template #acts>
        <a :href="d.send_url" target="_blank" rel="noopener" class="btn outline"><Icon name="external" :size="14" />{{ t('channel.send_guide') }}</a>
        <details class="pop">
          <summary class="btn outline"><Icon name="dash" :size="14" />{{ t('channel.props') }}</summary>
          <div class="pop-body">
            <div class="pop-row"><div class="k">{{ t('channel.prop_id') }}</div><div class="v mono">{{ d.channel.id }}</div></div>
            <div class="pop-row"><div class="k">{{ t('channel.prop_token') }}</div><div class="v mono">{{ d.token }}</div></div>
            <div class="pop-row"><div class="k">{{ t('channel.prop_used') }}</div><div class="v">{{ ago(d.last_used_at, t) }} <span class="dim mono">{{ d.last_used_ip }}</span></div></div>
          </div>
        </details>
        <button type="button" class="btn outline danger" @click="purging = true"><Icon name="trash" :size="14" />{{ t('channel.purge') }}</button>
      </template>
    </PageHead>

    <form class="tools" style="align-self:center;width:100%;max-width:960px" @submit.prevent="search">
      <input v-model="q" class="input" :placeholder="t('msgs.search_ph')" style="flex:1">
      <button type="submit" class="btn outline"><Icon name="search" :size="14" />{{ t('common.search') }}</button>
      <button v-if="applied" type="button" class="btn ghost" @click="clear">{{ t('common.clear') }}</button>
      <span v-if="applied" class="dim" style="font-size:12.5px">{{ plural(t, items.length, 'channel.match_one', 'channel.match_count') }}</span>
    </form>

    <div ref="stream" class="stream">
      <div v-if="hasMore" style="text-align:center"><button type="button" class="btn ghost sm" @click="loadMessages(true)">{{ t('common.older') }}<Icon name="chev" :size="13" /></button></div>
      <Empty v-if="!items.length">{{ applied ? t('msgs.empty_query') : t('channel.empty') }}</Empty>
      <template v-for="(m, i) in items" :key="m.uid">
        <div v-if="!applied && dayOf(i)" class="day"><Badge kind="muted" :dot="false">{{ dayOf(i) }}</Badge></div>
        <div class="bubble">
          <div class="hd">
            <div class="t">{{ m.title || m.summary }}</div>
            <Badge kind="muted" :dot="false">{{ m.type }}</Badge>
            <Badge v-if="!m.read" kind="info">{{ t('common.unread') }}</Badge>
            <PushBadge :ok="m.push.ok" :total="m.push.total" />
            <span class="at">{{ hm(m.created_at) }}</span>
          </div>
          <MessageBody :m="m" :q="applied" />
          <ReplyReadonly :m="m" />
          <div class="ft"><router-link :to="'/admin/messages/' + m.uid" class="btn ghost sm">{{ t('channel.delivery_log') }}</router-link></div>
        </div>
      </template>
    </div>
  </template>
  <Confirm v-if="purging" :title="t('channel.purge')" :text="t('channel.purge_ask')" :action="t('channel.purge')" :busy="busy" @ok="purge" @cancel="purging = false" />
</template>
