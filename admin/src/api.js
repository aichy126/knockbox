// /admin/api/* 的调用层。信封与 /api/v1 一样：{code, msg, data}。
//
// 判断成败看 code，不看 HTTP 状态——被拒绝的请求也是 200。
// 唯一的例外是 401：会话没了。它单独标出来，让路由把人送回登录页。
export class ApiError extends Error {
  constructor(message, { errorCode = '', status = 0, unauthorized = false } = {}) {
    super(message)
    this.errorCode = errorCode
    this.status = status
    this.unauthorized = unauthorized
  }
}

export async function api(path, { method = 'GET', body } = {}) {
  const init = { method, credentials: 'same-origin', headers: {} }
  if (body !== undefined) {
    init.headers['Content-Type'] = 'application/json'
    init.body = JSON.stringify(body)
  }
  const r = await fetch('/admin/api/' + path, init)
  let j = null
  try { j = await r.json() } catch { /* 不是 JSON 就当没有 */ }
  if (r.status === 401) {
    throw new ApiError(j?.msg || 'unauthorized', { errorCode: j?.data?.error_code || 'admin.session_expired', status: 401, unauthorized: true })
  }
  if (!j || j.code !== 0) {
    throw new ApiError(j?.msg || `HTTP ${r.status}`, { errorCode: j?.data?.error_code || '', status: r.status })
  }
  return j.data
}

export const get = (path) => api(path)
export const post = (path, body = {}) => api(path, { method: 'POST', body })
export const put = (path, body = {}) => api(path, { method: 'PUT', body })

// 拼查询串，空值不带。
export const qs = (params) => {
  const u = new URLSearchParams()
  for (const [k, v] of Object.entries(params)) if (v !== undefined && v !== null && v !== '') u.set(k, v)
  const s = u.toString()
  return s ? '?' + s : ''
}
