// 当前登录者 + 这台服务器。外壳要用，一次拿全。
import { ref } from 'vue'
import { get } from './api'

export const me = ref(null)

// 这次页面加载里，会话状态问过服务器了没有（登着或没登着都算问过）。
// 路由守卫进后台前先探一次 /me，没登着就把人送到登录页——登录页再探一遍是白跑一趟，
// 而它正好落在「什么都还没画出来」的那段空白里。
export const meProbed = ref(false)

export async function loadMe() {
  try {
    me.value = await get('me')
    return me.value
  } finally {
    meProbed.value = true
  }
}

export function clearMe() { me.value = null }
