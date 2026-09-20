// 当前登录者 + 这台服务器。外壳要用，一次拿全。
import { ref } from 'vue'
import { get } from './api'

export const me = ref(null)

export async function loadMe() {
  me.value = await get('me')
  return me.value
}

export function clearMe() { me.value = null }
