<script setup>
import { ref, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute } from 'vue-router'
import { get, post, qs } from '../api'
import { onUnauthorized } from '../router'
import { setCrumbs } from '../crumbs'
import { ago } from '../time'
import PageHead from '../components/PageHead.vue'
import Badge from '../components/Badge.vue'
import Empty from '../components/Empty.vue'
import Icon from '../components/Icon.vue'

const { t } = useI18n()
const route = useRoute()
const q = ref('')
const items = ref([])
const total = ref(0)
const hasMore = ref(false)
const cursor = ref('')
const err = ref('')

async function load(more = false) {
  err.value = ''
  try {
    const d = await get('members' + qs({ q: q.value, cursor: more ? cursor.value : '' }))
    items.value = more ? items.value.concat(d.items) : d.items
    total.value = d.total ?? items.value.length
    hasMore.value = d.has_more
    cursor.value = d.next_cursor || ''
  } catch (e) { if (!onUnauthorized(e, route)) err.value = e.message }
}

async function toggle(m) {
  try {
    const d = await post(`members/${m.id}/unlimited`, { on: !m.unlimited })
    m.unlimited = d.unlimited
  } catch (e) { if (!onUnauthorized(e, route)) err.value = e.message }
}

onMounted(() => { setCrumbs([{ text: t('nav.users') }]); load() })
</script>

<template>
  <PageHead :title="t('nav.users')">
    <template #sub>{{ t('users.sub', [total]) }}</template>
    <template #acts><router-link to="/admin/pair" class="btn"><Icon name="qr" :size="14" />{{ t('nav.pair') }}</router-link></template>
  </PageHead>
  <form class="tools" @submit.prevent="load()">
    <input v-model="q" class="input" :placeholder="t('users.search_ph')" style="width:260px">
    <button type="submit" class="btn outline">{{ t('common.search') }}</button>
    <button v-if="q" type="button" class="btn ghost" @click="q = ''; load()">{{ t('users.clear') }}</button>
  </form>
  <div v-if="err" class="err">{{ err }}</div>
  <div class="card">
    <Empty v-if="!items.length">{{ q ? t('users.empty_query') : t('users.empty_all') }}</Empty>
    <table v-else>
      <thead><tr><th>{{ t('common.name') }}</th><th>{{ t('common.role') }}</th><th>{{ t('common.device') }}</th><th>{{ t('common.channel') }}</th><th>{{ t('common.messages') }}</th><th>{{ t('users.col_login') }}</th><th>{{ t('users.col_quota') }}</th></tr></thead>
      <tbody>
        <tr v-for="m in items" :key="m.id">
          <td style="font-weight:500"><router-link :to="'/admin/users/' + m.id">{{ m.name }}</router-link></td>
          <td><Badge :kind="m.role === 'admin' ? 'info' : 'muted'" :dot="false">{{ m.role === 'admin' ? t('common.role_admin') : t('common.role_member') }}</Badge></td>
          <td class="num">{{ m.devices }}</td><td class="num">{{ m.channels }}</td><td class="num">{{ m.messages }}</td>
          <td class="dim">{{ ago(m.last_login_at, t) }}</td>
          <td class="r"><button type="button" class="btn sm" :class="m.unlimited ? 'outline' : 'ghost'" @click="toggle(m)">{{ m.unlimited ? t('users.unlimited_on') : t('users.unlimited') }}</button></td>
        </tr>
      </tbody>
    </table>
    <div v-if="hasMore" class="pager2">
      <button type="button" class="btn outline sm" @click="load(true)">{{ t('common.next_page') }}<Icon name="chev" :size="13" /></button>
    </div>
  </div>
  <div class="note"><Icon name="alert" :size="14" /><div v-html="t('users.note')" /></div>
</template>
