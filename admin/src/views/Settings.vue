<script setup>
import { ref, onMounted, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import { get, put, post } from '../api'
import { me, loadMe, clearMe } from '../store'
import { onUnauthorized } from '../router'
import { setCrumbs } from '../crumbs'

const { t } = useI18n()
const route = useRoute()
const router = useRouter()
// tab 的值是 key 不是文案：文案一翻译，中文当路由状态的那条链接就点不亮了。
const tab = computed(() => (route.query.tab === 'server' ? 'server' : 'public'))
const s = ref(null)
const form = ref(null)
const err = ref('')
const saved = ref(false)
const busy = ref(false)
const pw = ref({ current: '', new: '', confirm: '' })
const pwErr = ref('')

async function load() {
  err.value = ''
  try { s.value = await get('settings'); form.value = { ...s.value.values } }
  catch (e) { if (!onUnauthorized(e, route)) err.value = e.message }
}
async function save() {
  err.value = ''; saved.value = false; busy.value = true
  try {
    const f = form.value
    s.value = await put('settings', {
      public_enabled: !!f.public_enabled, register_per_hour: Number(f.register_per_hour),
      max_channels: Number(f.max_channels), max_per_day: Number(f.max_per_day),
      retention_days: Number(f.retention_days), site_name: f.site_name,
    })
    form.value = { ...s.value.values }
    saved.value = true
    await loadMe()
  } catch (e) { if (!onUnauthorized(e, route)) err.value = e.message }
  finally { busy.value = false }
}
async function changePassword() {
  pwErr.value = ''; busy.value = true
  try {
    await post('account/password', pw.value)
    // 改完这个账号的会话全部失效，人被带回登录页，提示留在那边。
    clearMe()
    router.push({ name: 'login', query: { changed: 1 } })
  } catch (e) { if (!onUnauthorized(e, route)) pwErr.value = e.message }
  finally { busy.value = false }
}
const changed = (k) => s.value && !s.value.from_config[k]

const cli = computed(() => [
  ['knockbox user add <name>', t('settings.cli_add')],
  ['knockbox user passwd <name>', t('settings.cli_passwd')],
  ['knockbox user list', t('settings.cli_list')],
  ['knockbox user disable <name>', t('settings.cli_disable')],
])

onMounted(() => { setCrumbs([{ text: t('nav.settings') }]); load() })
</script>

<template>
  <div class="ph"><div><h1>{{ t('nav.settings') }}</h1><div class="sub">{{ t('settings.sub') }}</div></div></div>
  <div class="seg">
    <router-link :to="{ query: {} }" class="tab" :class="{ on: tab === 'public' }">{{ t('settings.tab_public') }}</router-link>
    <router-link :to="{ query: { tab: 'server' } }" class="tab" :class="{ on: tab === 'server' }">{{ t('settings.tab_server') }}</router-link>
  </div>
  <div v-if="err" class="err">{{ err }}</div>

  <form v-if="tab === 'public' && form" style="display:flex;flex-direction:column;gap:16px" @submit.prevent="save">
    <div v-if="saved" class="note"><div>{{ t('settings.saved') }}</div></div>
    <div class="grid2">
      <div class="card">
        <div class="card-h"><div class="t">{{ t('settings.tab_public') }}</div></div>
        <div class="set-row"><div class="lab"><b>{{ t('settings.public_mode') }}</b><span>{{ t('settings.public_hint') }}</span></div><div class="ctl"><span v-if="changed('public_enabled')" class="dot-set" :title="t('settings.dot_title')" /><button type="button" class="toggle" :class="{ on: form.public_enabled }" role="switch" :aria-checked="form.public_enabled" @click="form.public_enabled = !form.public_enabled" /></div></div>
        <div class="set-row"><div class="lab"><b>{{ t('settings.reg_per_hour') }}</b><span>{{ t('settings.reg_hint') }}</span></div><div class="ctl"><span v-if="changed('register_per_hour')" class="dot-set" /><input v-model="form.register_per_hour" class="input" type="number" min="0"></div></div>
        <div class="set-row"><div class="lab"><b>{{ t('settings.site_name') }}</b><span>{{ t('settings.site_hint') }}</span></div><div class="ctl"><span v-if="changed('site_name')" class="dot-set" /><input v-model="form.site_name" class="input wide" maxlength="64"></div></div>
      </div>
      <div class="card">
        <div class="card-h"><div class="t">{{ t('settings.card_quota') }}</div></div>
        <div class="set-row"><div class="lab"><b>{{ t('settings.max_channels') }}</b><span>{{ t('settings.max_ch_hint') }}</span></div><div class="ctl"><span v-if="changed('max_channels')" class="dot-set" /><input v-model="form.max_channels" class="input" type="number" min="0"></div></div>
        <div class="set-row"><div class="lab"><b>{{ t('settings.max_per_day') }}</b><span>{{ t('settings.max_day_hint') }}</span></div><div class="ctl"><span v-if="changed('max_per_day')" class="dot-set" /><input v-model="form.max_per_day" class="input" type="number" min="0"></div></div>
        <div class="set-row"><div class="lab"><b>{{ t('settings.retention') }}</b><span>{{ t('settings.ret_hint') }}</span></div><div class="ctl"><span v-if="changed('retention_days')" class="dot-set" /><input v-model="form.retention_days" class="input" type="number" min="0"></div></div>
        <div class="card-foot" v-html="t('settings.quota_note')" />
      </div>
    </div>
    <div style="display:flex;align-items:center;gap:12px">
      <button type="submit" class="btn" :disabled="busy">{{ t('common.save') }}</button>
      <span class="dim" style="font-size:12.5px" v-html="t('settings.save_hint', ['<span class=&quot;dot-set&quot; style=&quot;vertical-align:middle&quot;></span>'])" />
    </div>
  </form>

  <div v-if="tab === 'server' && me" class="grid2">
    <div class="col">
      <div class="card">
        <div class="card-h"><div class="t">{{ t('settings.card_server') }}</div></div>
        <div class="card-b kv">
          <div class="k">{{ t('settings.info_name') }}</div><div class="v">{{ me.server.name }}</div>
          <div class="k">{{ t('settings.info_url') }}</div><div class="v mono">{{ me.server.external_url }}</div>
          <div class="k">{{ t('settings.info_version') }}</div><div class="v mono">{{ me.server.version }}</div>
          <div class="k">{{ t('settings.info_mode') }}</div><div class="v"><span class="badge" :class="me.server.public_enabled ? 'info' : 'muted'">{{ me.server.public_enabled ? t('settings.mode_public') : t('settings.mode_self') }}</span></div>
        </div>
      </div>
      <div class="card">
        <div class="card-h"><div class="t">{{ t('settings.card_usage') }}</div></div>
        <div class="kvgrid">
          <div><div class="k">{{ t('settings.usage_channels') }}</div><div class="v">{{ me.usage.channels }}</div></div>
          <div><div class="k">{{ t('settings.usage_today') }}</div><div class="v">{{ me.usage.today }}</div></div>
          <div><div class="k">{{ t('settings.usage_total') }}</div><div class="v">{{ me.usage.messages }}</div></div>
          <div><div class="k">{{ t('settings.usage_files') }}</div><div class="v">{{ (me.usage.file_bytes / 1e6).toFixed(1) }} MB</div></div>
        </div>
      </div>
      <div class="card">
        <div class="card-h"><div class="t">{{ t('settings.card_cli') }}</div></div>
        <div class="card-b">
          <p style="margin:0 0 10px;line-height:1.7;font-size:13.5px">{{ t('settings.cli_intro') }}</p>
          <div class="code">
            <div v-for="(c, i) in cli" :key="i" class="cli-row"><span>{{ c[0] }}</span><span class="c">{{ c[1] }}</span></div>
          </div>
        </div>
      </div>
    </div>
    <div class="col">
      <div class="card">
        <div class="card-h"><div class="t">{{ t('settings.pw_card', [me.user.username]) }}</div></div>
        <form class="card-b" style="display:flex;flex-direction:column;gap:12px;padding-top:4px" @submit.prevent="changePassword">
          <div v-if="pwErr" class="err">{{ pwErr }}</div>
          <div class="field"><label for="pw0">{{ t('settings.pw_current') }}</label><input id="pw0" v-model="pw.current" class="input" type="password" autocomplete="current-password" required></div>
          <div class="grid2" style="gap:12px">
            <div class="field"><label for="pw1">{{ t('settings.pw_new') }}</label><input id="pw1" v-model="pw.new" class="input" type="password" autocomplete="new-password" required></div>
            <div class="field"><label for="pw2">{{ t('settings.pw_confirm') }}</label><input id="pw2" v-model="pw.confirm" class="input" type="password" autocomplete="new-password" required></div>
          </div>
          <div><button type="submit" class="btn" :disabled="busy">{{ t('settings.pw_submit') }}</button></div>
        </form>
        <div class="card-foot">{{ t('settings.pw_note') }}</div>
      </div>
    </div>
  </div>
</template>
