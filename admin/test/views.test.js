// 守门员：英文界面里不该出现一个汉字。
//
// 文案是从语料取的，漏一句在中文下完全看不出来——它本来就是中文。只有拿英文渲染
// 每一屏再扫汉字，漏掉的那句才会自己跳出来。也防以后：谁再往视图里写死一句中文，这里立刻红。
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createRouter, createMemoryHistory } from 'vue-router'
import { i18n, setLang } from '../src/i18n'
import { me } from '../src/store'

const now = Math.floor(Date.now() / 1000)
const msg = (over = {}) => ({
  id: 1, uid: 'U1', user_id: 1, owner: 'aichy', channel: 'C1', channel_name: 'Deploys', type: 'text',
  title: 'Build #1832 failed', summary: 'main · 42s', body: 'main · 42s', extra: '', created_at: now,
  read: false, push: { ok: 2, total: 2 }, replyable: false, ...over,
})
const replyMsg = msg({ replyable: true, extra: JSON.stringify({ reply: { type: 'choice', options: ['Open', 'Keep closed'] } }), reply: 'Keep closed', replied_at: now })

const DATA = {
  me: { user: { id: 1, name: 'aichy', username: 'aichy', role: 'admin', unlimited: true },
    usage: { channels: 4, channel_limit: 20, today: 128, daily_limit: 500, messages: 1188, file_bytes: 342e6, retention_days: 30, unlimited: true },
    server: { name: 'My Push', version: 'v1.1.0', external_url: 'https://push.example.com', host: 'push.example.com', public_enabled: false } },
  overview: { window: 86400, messages: { current: 128, prev: 96 }, push: { ok: 2, failed: 2, retrying: 1, rate: 98.4 }, devices: { pushable: 3, sandbox: 1 },
    channels: { total: 4, muted: 1 }, failures: [{ reason: 'Unregistered', http_status: 410, count: 2 }], recent: [msg()] },
  members: { items: [{ id: 1, name: 'aichy', role: 'admin', status: 1, unlimited: true, devices: 2, channels: 4, messages: 1188, last_login_at: now, created_at: now }], has_more: true, next_cursor: '1', total: 3 },
  'members/1': { member: { id: 1, name: 'aichy', role: 'admin', status: 1, unlimited: false, devices: 1, channels: 1, messages: 1188, last_login_at: now, created_at: now },
    usage: DATA_USAGE(), channels: [{ id: 'C1', name: 'Deploys', meta: '', muted: false, mute_until: now + 3600, sound: 'default', level: 'active', messages: 388, last_msg_at: now }],
    devices: [{ id: 5, name: "Amy's iPhone", platform: 'iOS', model: 'iPhone18,1', os_version: '26.0', app_version: '1.1.1', apns_env: 'production', can_push: true, status: 1, sync_rev: 10482, last_seen_at: now }] },
  'channels/C1': { channel: { id: 'C1', name: 'Deploys', meta: '', muted: false, mute_until: 0, sound: 'default', level: 'active', messages: 2, last_msg_at: now },
    owner: { id: 1, name: 'aichy' }, token: 'ch_x', last_used_at: now, last_used_ip: '10.0.0.1', send_url: 'https://push.example.com/s/ch_x' },
  'channels/C1/messages': { items: [msg(), replyMsg], has_more: true, next_cursor: '1' },
  'messages/U1': { message: replyMsg, reply_webhook: 'https://home.example.com/hook',
    push_log: [{ status: 1, http_status: 200, reason: '', attempts: 1, apns_id: 'A', updated_at: now, device: 'iPhone', apns_env: 'production' }],
    reply_hooks: [{ status: 1, attempt: 1, status_code: 200, error: '', updated_at: now }] },
  'pair/targets': { items: [{ id: 1, name: 'aichy', is_me: true }], count: 1 },
  settings: { values: { public_enabled: true, register_per_hour: 3, max_channels: 20, max_per_day: 500, retention_days: 30, site_name: 'Knockbox' }, from_config: { public_enabled: false, register_per_hour: true, max_channels: true, max_per_day: true, retention_days: false, site_name: true } },
}
function DATA_USAGE() { return { channels: 4, channel_limit: 20, today: 128, daily_limit: 500, messages: 1188, file_bytes: 342e6, retention_days: 30, unlimited: false } }

vi.mock('../src/api', () => ({
  get: vi.fn(async (path) => { const k = path.split('?')[0]; if (!(k in DATA)) throw new Error('no fixture for ' + k); return structuredClone(DATA[k]) }),
  post: vi.fn(async () => ({})),
  put: vi.fn(async () => structuredClone(DATA.settings)),
  qs: (p) => { const u = new URLSearchParams(); for (const [k, v] of Object.entries(p)) if (v) u.set(k, v); const s = u.toString(); return s ? '?' + s : '' },
}))

const views = {
  Overview: ['/admin', () => import('../src/views/Overview.vue')],
  Members: ['/admin/users', () => import('../src/views/Members.vue')],
  Member: ['/admin/users/1', () => import('../src/views/Member.vue')],
  Channel: ['/admin/channels/C1', () => import('../src/views/Channel.vue')],
  Message: ['/admin/messages/U1', () => import('../src/views/Message.vue')],
  Pair: ['/admin/pair', () => import('../src/views/Pair.vue')],
  Settings: ['/admin/settings', () => import('../src/views/Settings.vue')],
  SettingsServer: ['/admin/settings?tab=server', () => import('../src/views/Settings.vue')],
  Login: ['/login', () => import('../src/views/Login.vue')],
  Shell: ['/admin', () => import('../src/components/Shell.vue')],
}

async function render(path, loader) {
  const router = createRouter({ history: createMemoryHistory(), routes: [
    { path: '/login', name: 'login', component: { template: '<div/>' } },
    { path: '/admin', name: 'dash', component: { template: '<div/>' } },
    { path: '/admin/users', name: 'users', component: { template: '<div/>' } },
    { path: '/admin/users/:id', name: 'member', component: { template: '<div/>' } },
    { path: '/admin/channels/:id', name: 'channel', component: { template: '<div/>' } },
    { path: '/admin/messages/:uid', name: 'message', component: { template: '<div/>' } },
    { path: '/admin/pair', name: 'pair', component: { template: '<div/>' } },
    { path: '/admin/settings', name: 'settings', component: { template: '<div/>' } },
  ] })
  await router.push(path)
  await router.isReady()
  const { default: C } = await loader()
  const w = mount(C, { global: { plugins: [router, i18n] }, attachTo: document.body })
  await flushPromises()
  await flushPromises()
  return w
}

describe('English UI has no Chinese characters', () => {
  beforeEach(() => { setLang('en'); me.value = structuredClone(DATA.me) })
  for (const [name, [path, loader]] of Object.entries(views)) {
    it(name, async () => {
      const w = await render(path, loader)
      // 语言开关显示的是【别的语言自己的名字】，所以英文界面上那个按钮
      // 写着「中文」是对的，不是漏翻。扫描前把它摘掉。
      const clone = w.element.cloneNode(true)
      clone.querySelectorAll('.langsw').forEach((el) => el.remove())
      const text = clone.textContent
      expect(text.length, 'rendered nothing').toBeGreaterThan(20)
      const han = text.match(/[一-鿿]/g)
      expect(han, `Chinese leaked into ${name}: …${text.slice(Math.max(0, text.search(/[一-鿿]/) - 40), text.search(/[一-鿿]/) + 40)}…`).toBeNull()
      w.unmount()
    })
  }
  it('and Chinese really switches', async () => {
    setLang('zh')
    const w = await render('/admin/users', views.Members[1])
    expect(w.text()).toContain('成员')
    w.unmount()
  })
})

// 面包屑真的画出来了。
//
// script setup 里 import 进来的 ref 在模板中【已经解包】，写 crumbs.value
// 拿到的是 undefined——页面照常渲染，只是顶栏左边空着，不报任何错。
describe('breadcrumbs', () => {
  it('renders what a view set', async () => {
    setLang('en')
    me.value = structuredClone(DATA.me)
    const { setCrumbs } = await import('../src/crumbs')
    setCrumbs([{ text: 'Members', to: '/admin/users' }, { text: 'aichy' }])
    const w = await render('/admin', views.Shell[1])
    const crumb = w.find('.crumb')
    expect(crumb.exists()).toBe(true)
    expect(crumb.text()).toContain('Members')
    expect(crumb.text()).toContain('aichy')
    w.unmount()
  })
})
