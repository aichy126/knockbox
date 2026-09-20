<script setup>
import { ref, onMounted, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute } from 'vue-router'
import { get, post } from '../api'
import { me } from '../store'
import { onUnauthorized } from '../router'
import { setCrumbs } from '../crumbs'
import { plural } from '../i18n'
import PageHead from '../components/PageHead.vue'
import Icon from '../components/Icon.vue'

const { t } = useI18n()
const route = useRoute()
const targets = ref([])
const name = ref('')
const issued = ref(null)
const err = ref('')
const busy = ref(false)

// 有效期跟着 server.pair_ttl 走，不写死：配置改了而文案不改，那一句就开始骗人。
const minutes = computed(() => (issued.value ? Math.max(1, Math.round((issued.value.expires_at - Date.now() / 1000) / 60)) : 0))

onMounted(async () => {
  setCrumbs([{ text: t('nav.pair') }])
  try {
    const d = await get('pair/targets')
    targets.value = d.items
    name.value = d.items.find((x) => x.is_me)?.name || me.value?.user.name || ''
  } catch (e) { if (!onUnauthorized(e, route)) err.value = e.message }
})

// 名字能对上就给那个人，对不上就当是要新建一个——
// 让用户先去别处建人、再回来选，是没必要的一次往返。
async function issue() {
  err.value = ''; busy.value = true
  try {
    const n = name.value.trim()
    const hit = targets.value.find((x) => x.name === n)
    issued.value = await post('pair', hit ? { member_id: hit.id } : { new_name: n })
    if (issued.value.member.created) { const d = await get('pair/targets'); targets.value = d.items }
  } catch (e) { if (!onUnauthorized(e, route)) err.value = e.message }
  finally { busy.value = false }
}
</script>

<template>
  <PageHead :title="t('nav.pair')"><template #sub>{{ t('pair.sub') }}</template></PageHead>
  <div v-if="err" class="err">{{ err }}</div>
  <div class="card">
    <div class="card-h"><div class="t">{{ t('pair.card_who') }}</div></div>
    <form class="card-b" style="padding-top:2px" @submit.prevent="issue">
      <div class="tools">
        <span class="dim">{{ t('pair.send_to') }}</span>
        <span class="picker">
          <input v-model="name" class="input" list="pair-targets" :placeholder="t('pair.name_ph')" autocomplete="off" style="width:220px">
          <datalist id="pair-targets"><option v-for="x in targets" :key="x.id" :value="x.name" /></datalist>
          <span class="hint">{{ plural(t, targets.length, 'common.people_one', 'common.people_count') }}</span>
        </span>
        <button type="submit" class="btn" :disabled="busy || !name.trim()"><Icon name="qr" :size="14" />{{ t('pair.issue') }}</button>
      </div>
      <div class="dim" style="font-size:12.5px;margin-top:10px">{{ t('pair.hint') }}</div>
    </form>
  </div>
  <div v-if="issued" class="card">
    <div class="card-h"><div class="t">{{ t('pair.card_scan') }}</div></div>
    <div class="card-b two-col" style="display:grid;grid-template-columns:300px minmax(0,1fr);gap:28px;align-items:start;padding-top:6px">
      <div style="display:flex;flex-direction:column;align-items:center;gap:12px">
        <div class="qr" v-html="issued.qr_svg || `<div class='dim'>${t('pair.qr_fail')}</div>`" />
        <div class="bigcode">{{ issued.code }}</div>
        <div class="mono dim" style="font-size:12.5px">{{ me?.server.external_url }}</div>
      </div>
      <div style="display:flex;flex-direction:column;gap:14px;padding-top:6px">
        <div class="steps">
          <div class="step"><b>1</b><div>{{ t('pair.step1') }}</div></div>
          <div class="step"><b>2</b><div>{{ t('pair.step2') }}</div></div>
          <div class="step"><b>3</b><div v-html="t('pair.step3', [issued.member.name])" /></div>
        </div>
        <div class="note"><Icon name="shield" :size="14" /><div>{{ t('pair.note', [minutes]) }}</div></div>
      </div>
    </div>
  </div>
</template>
