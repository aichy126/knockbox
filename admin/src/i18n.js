// 文案只有一份真源：internal/api/web/locales/{en,zh}.json，和服务端共用。
// 这里只做两件事：把 Go 的 %d/%s 换成 vue-i18n 的 {0} {1}，以及决定用哪门语言。
import { createI18n } from 'vue-i18n'
import zh from '@web/locales/zh.json'
import en from '@web/locales/en.json'

const convert = (v) => {
  if (typeof v === 'string') { let i = 0; return v.replace(/%[sd]/g, () => `{${i++}}`) }
  if (v && typeof v === 'object') return Object.fromEntries(Object.entries(v).map(([k, x]) => [k, convert(x)]))
  return v
}

export const LANGS = { en, zh }
export const LANG_KEYS = Object.keys(LANGS)
export const COOKIE = 'lang'

const messages = Object.fromEntries(Object.entries(LANGS).map(([k, raw]) => [k, {
  ...convert(raw.admin),
  lang_name: raw.lang_name,
  html_lang: raw.html_lang || k,
}]))

const cookie = () => document.cookie.split('; ').find((c) => c.startsWith(COOKIE + '='))?.slice(COOKIE.length + 1)

// 顺序和服务端一样：cookie → 浏览器语言 → 英文。
export function pickLang() {
  const c = cookie()
  if (c && LANGS[c]) return c
  const nav = (navigator.language || '').toLowerCase()
  const hit = LANG_KEYS.find((k) => nav === k || nav.startsWith(k + '-'))
  return hit || 'en'
}

export const i18n = createI18n({
  legacy: false,
  locale: pickLang(),
  fallbackLocale: 'en',
  messages,
  warnHtmlMessage: false, // 几句语料自带 <b> 与链接，是我们自己写的
})

// 切语言：记进 cookie（一年），服务端的错误句子也按它渲染。
export function setLang(k) {
  if (!LANGS[k]) return
  i18n.global.locale.value = k
  document.cookie = `${COOKIE}=${k}; Path=/; Max-Age=31536000; SameSite=Lax`
  document.documentElement.lang = messages[k].html_lang
}

// 开关上写的是【另一门】语言自己的名字：中文用户看不懂 "Language"，反过来也一样。
export const otherLang = () => LANG_KEYS.find((k) => k !== i18n.global.locale.value) || 'en'
export const langName = (k) => messages[k]?.lang_name || k

// 英语把 1 和其余分开，中文不分——两条写成一样的就行。
export const plural = (t, n, one, other) => (n === 1 ? t(one, [n]) : t(other, [n]))

document.documentElement.lang = messages[i18n.global.locale.value].html_lang
