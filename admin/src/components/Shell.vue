<script setup>
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import { me, clearMe } from '../store'
import { post } from '../api'
import { mark } from '../icons'
import Icon from './Icon.vue'
import LangSwitch from './LangSwitch.vue'
import { crumbs } from '../crumbs'

const { t } = useI18n()
const route = useRoute()
const router = useRouter()

// 导航按【归属关系】分：消息属于频道，频道和设备属于成员，所以顶层只有成员。
const NAV = [
  ['dash', '/admin', 'dash'],
  ['users', '/admin/users', 'users'],
  ['pair', '/admin/pair', 'qr'],
  ['settings', '/admin/settings', 'gear'],
]
const active = () => ({ dash: 'dash', users: 'users', member: 'users', channel: 'users', message: 'users', pair: 'pair', settings: 'settings' })[route.name]

async function signOut() {
  try { await post('logout') } catch { /* 会话本来就没了也一样回登录页 */ }
  clearMe()
  router.push({ name: 'login' })
}
</script>

<template>
  <div class="app">
    <aside class="sidebar">
      <div class="brand">
        <div class="brand-mark" v-html="mark" />
        <div><div class="brand-name">Knockbox</div><div class="brand-sub">{{ me?.server.name }}</div></div>
      </div>
      <nav class="nav">
        <router-link v-for="[id, to, ic] in NAV" :key="id" :to="to" class="item" :class="{ on: active() === id }">
          <Icon :name="ic" :size="16" /><span>{{ t('nav.' + id) }}</span>
        </router-link>
      </nav>
      <div class="side-foot"><span class="dot" style="background:var(--success)" /><span>{{ t('nav.online') }} · {{ me?.server.version }}</span></div>
    </aside>
    <div class="main">
      <header class="topbar">
        <nav class="crumb">
          <template v-for="(c, i) in crumbs" :key="i">
            <span v-if="i === crumbs.length - 1" class="cur">{{ c.text }}</span>
            <template v-else>
              <router-link v-if="c.to" :to="c.to">{{ c.text }}</router-link>
              <span v-else>{{ c.text }}</span>
              <Icon name="chev" :size="14" />
            </template>
          </template>
        </nav>
        <div class="spacer" />
        <div class="tools">
          <LangSwitch />
          <span class="who"><i>{{ (me?.user.username || '?')[0].toUpperCase() }}</i>{{ me?.user.username }}</span>
          <button type="button" class="btn ghost sm" @click="signOut"><Icon name="logout" :size="14" />{{ t('nav.logout') }}</button>
        </div>
      </header>
      <div class="content">
        <slot />
      </div>
    </div>
  </div>
</template>
