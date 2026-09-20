<script setup>
import { ref, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import { post } from '../api'
import { me, loadMe, meProbed } from '../store'
import { mark } from '../icons'
import LangSwitch from '../components/LangSwitch.vue'

const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const username = ref('')
const password = ref('')
const err = ref('')
const busy = ref(false)
const server = ref(null)

// 已经登着就别停在这一页。
//
// 服务器名走 /api/v1/server：它免鉴权，本来就是给「配对前探一下这个地址对不对」
// 用的。拿 /admin/api/me 的话未登录时是 401，品牌下面那行永远是空的。
onMounted(async () => {
  try {
    const r = await fetch('/api/v1/server', { credentials: 'same-origin' })
    const j = await r.json()
    if (j?.code === 0) server.value = { name: j.data.name, host: location.host }
  } catch { /* 拿不到就不显示，不挡登录 */ }
  // 被守卫送过来的人，会话状态刚刚问过了，别再问一遍。
  // 直接打开 /login 的人没问过——守卫对登录页是放行的，所以这里要补上这一问。
  if (meProbed.value) {
    if (me.value) router.replace(route.query.next || '/admin')
    return
  }
  try {
    await loadMe()
    router.replace(route.query.next || '/admin')
  } catch { /* 没登录，正常 */ }
})

async function submit() {
  err.value = ''
  busy.value = true
  try {
    await post('login', { username: username.value, password: password.value })
    await loadMe()
    router.replace(route.query.next || '/admin')
  } catch (e) {
    err.value = e.message
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <div class="login">
    <LangSwitch fixed />
    <form @submit.prevent="submit">
      <div style="display:flex;align-items:center;gap:10px;margin-bottom:6px">
        <div class="brand-mark" v-html="mark" />
        <div><div class="brand-name">Knockbox</div><div v-if="server" class="brand-sub">{{ server.name }} · {{ server.host }}</div></div>
      </div>
      <div v-if="route.query.changed" class="note"><div>{{ t('login.changed') }}</div></div>
      <div v-if="err" class="err">{{ err }}</div>
      <div class="field"><label for="u">{{ t('login.username') }}</label><input id="u" v-model="username" class="input" autocomplete="username" autofocus required style="height:38px"></div>
      <div class="field"><label for="p">{{ t('login.password') }}</label><input id="p" v-model="password" class="input" type="password" autocomplete="current-password" required style="height:38px"></div>
      <button class="btn" type="submit" :disabled="busy" style="height:38px;width:100%;margin-top:4px">{{ t('login.title') }}</button>
    </form>
  </div>
</template>
