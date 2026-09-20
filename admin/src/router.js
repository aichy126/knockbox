import { createRouter, createWebHistory } from 'vue-router'
import { me, loadMe } from './store'
// 登录页不走懒加载：它是唯一一个「还没登录就会用到」的页面，拆成分片等于在
// 那段什么都没画的空白里再加一次往返。其余页面都在登录之后，照旧按需拿。
import Login from './views/Login.vue'

const routes = [
  { path: '/login', name: 'login', component: Login },
  { path: '/admin', name: 'dash', component: () => import('./views/Overview.vue') },
  { path: '/admin/users', name: 'users', component: () => import('./views/Members.vue') },
  { path: '/admin/users/:id', name: 'member', component: () => import('./views/Member.vue') },
  { path: '/admin/channels/:id', name: 'channel', component: () => import('./views/Channel.vue') },
  { path: '/admin/messages/:uid', name: 'message', component: () => import('./views/Message.vue') },
  { path: '/admin/pair', name: 'pair', component: () => import('./views/Pair.vue') },
  { path: '/admin/settings', name: 'settings', component: () => import('./views/Settings.vue') },
  // 服务端直出那一版留下的地址，进来了就带到对应的新页。
  { path: '/admin/messages', redirect: '/admin' },
  { path: '/:rest(.*)', redirect: '/admin' },
]

export const router = createRouter({ history: createWebHistory('/'), routes })

// 进后台前先确认会话还在：没了就去登录页，登录完回到本来要去的地方。
router.beforeEach(async (to) => {
  if (to.name === 'login') return true
  if (me.value) return true
  try {
    await loadMe()
    return true
  } catch (e) {
    if (e?.unauthorized) return { name: 'login', query: { next: to.fullPath } }
    throw e
  }
})

// 任何视图里撞到 401 都走这一条，不用各自猜。
export function onUnauthorized(e, route) {
  if (e?.unauthorized) {
    router.push({ name: 'login', query: { next: route?.fullPath } })
    return true
  }
  return false
}
