// 时间的几种写法。t 是 vue-i18n 的 t。
const pad = (n) => String(n).padStart(2, '0')
const d = (ts) => new Date(ts * 1000)

export const clock = (ts) => {
  if (!ts) return '—'
  const x = d(ts)
  if (Date.now() - ts * 1000 < 86400e3) return `${pad(x.getHours())}:${pad(x.getMinutes())}`
  return `${pad(x.getMonth() + 1)}-${pad(x.getDate())} ${pad(x.getHours())}:${pad(x.getMinutes())}`
}
export const hm = (ts) => { const x = d(ts); return `${pad(x.getHours())}:${pad(x.getMinutes())}` }
export const hms = (ts) => { const x = d(ts); return `${hm(ts)}:${pad(x.getSeconds())}` }
export const ymd = (ts) => { const x = d(ts); return `${x.getFullYear()}-${pad(x.getMonth() + 1)}-${pad(x.getDate())}` }
export const full = (ts) => (ts ? `${ymd(ts)} ${hms(ts)}` : '—')

export const ago = (ts, t) => {
  if (!ts) return t('common.dash')
  const s = Math.max(0, Date.now() / 1000 - ts)
  if (s < 60) return t('time.just_now')
  if (s < 3600) return t('time.minutes_ago', [Math.floor(s / 60)])
  if (s < 86400) return t('time.hours_ago', [Math.floor(s / 3600)])
  if (s < 30 * 86400) return t('time.days_ago', [Math.floor(s / 86400)])
  return ymd(ts)
}

// 消息流里的日期分隔：今天 / 昨天 / 按本语言的版式。
export const dayLabel = (ts, t, locale) => {
  const x = d(ts), now = new Date()
  const start = new Date(now.getFullYear(), now.getMonth(), now.getDate()).getTime()
  if (x.getTime() >= start) return t('time.today')
  if (x.getTime() >= start - 86400e3) return t('time.yesterday')
  return x.toLocaleDateString(locale === 'zh' ? 'zh-CN' : 'en-US', { month: 'short', day: 'numeric' })
}
