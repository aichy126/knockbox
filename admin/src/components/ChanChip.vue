<script setup>
// 频道标。名字、图标、颜色是 app 写进 meta 的，服务端只存不读；
// 这里只认 color（十六进制）给一个色块，图标名是 SF Symbols 的，网页上对不上，不画。
import Icon from './Icon.vue'
const props = defineProps({ id: String, name: String, meta: { type: String, default: '' } })
const color = () => {
  try { const c = JSON.parse(props.meta || '{}').color; return /^#[0-9a-fA-F]{6}$/.test(c) ? c : '' } catch { return '' }
}
</script>

<template>
  <span class="chan">
    <i :style="color() ? { background: color() + '22', color: color() } : {}"><Icon name="hash" :size="12" /></i>
    <router-link :to="'/admin/channels/' + id">{{ name }}</router-link>
  </span>
</template>
