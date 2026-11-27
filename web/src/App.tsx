import { useEffect, useState } from 'react'

type Status = {
  authenticated: boolean
  role: 'guest' | 'admin'
  needs_setup: boolean
  username?: string
  session_expires_at?: string
}

type Health = {
  status: string
  version: string
  commit: string
  database: { status: string; schema_version: number }
  data: { writable: boolean }
  process: { uid: number; gid: number; is_root: boolean }
}

async function post(path: string, body: unknown) {
  const r = await fetch(path, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })
  let data: Record<string, unknown> = {}
  try {
    data = await r.json()
  } catch {
    /* 非 JSON 响应 */
  }
  return { ok: r.ok, status: r.status, data }
}

export default function App() {
  const [status, setStatus] = useState<Status | null>(null)
  const [health, setHealth] = useState<Health | null>(null)
  const [msg, setMsg] = useState('')
  const [busy, setBusy] = useState(false)

  const load = () => {
    fetch('/api/auth/status')
      .then((r) => r.json())
      .then(setStatus)
      .catch(() => setMsg('无法读取登录状态'))
    fetch('/health')
      .then((r) => r.json())
      .then(setHealth)
      .catch(() => undefined)
  }

  useEffect(load, [])

  async function login(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault()
    const form = new FormData(e.currentTarget)
    setBusy(true)
    setMsg('')
    const r = await post('/api/auth/login', {
      username: form.get('username'),
      password: form.get('password'),
    })
    setBusy(false)
    if (r.ok) {
      setMsg('')
      load()
    } else if (r.status === 429) {
      setMsg(`登录失败次数过多，请 ${r.data.retry_after_seconds ?? ''} 秒后重试`)
    } else {
      setMsg((r.data.message as string) || '登录失败')
    }
  }

  async function logout() {
    setBusy(true)
    await post('/api/auth/logout', {})
    setBusy(false)
    load()
  }

  async function changePassword(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault()
    const form = new FormData(e.currentTarget)
    setBusy(true)
    setMsg('')
    const r = await post('/api/auth/password', {
      current_password: form.get('current_password'),
      new_password: form.get('new_password'),
    })
    setBusy(false)
    setMsg(
      r.ok
        ? '密码已修改，其他设备的登录会话已失效'
        : (r.data.message as string) || '修改失败',
    )
    if (r.ok) {
      ;(e.target as HTMLFormElement).reset()
    }
  }

  return (
    <main className="page">
      <section className="card">
        <h1>LocalHub</h1>
        <p className="muted">P1.1 账户与权限基础：单管理员登录、访客只读公开页面。</p>

        {msg && <p className="bad">{msg}</p>}

        {!status && <p className="muted">正在读取状态…</p>}

        {status && !status.authenticated && (
          <>
            {status.needs_setup ? (
              <p className="warn">
                管理员尚未初始化：请设置环境变量 LOCALHUB_ADMIN_PASSWORD（12-72 字节）后重启容器。
              </p>
            ) : (
              <form onSubmit={login} className="form">
                <label>
                  用户名
                  <input name="username" defaultValue="admin" autoComplete="username" />
                </label>
                <label>
                  密码
                  <input name="password" type="password" autoComplete="current-password" required />
                </label>
                <button disabled={busy}>登录</button>
              </form>
            )}
            <p className="muted">访客可直接访问本页与 /health，管理员接口一律由后端拒绝。</p>
          </>
        )}

        {status && status.authenticated && (
          <>
            <p>
              已登录：<strong>{status.username}</strong>（管理员）
            </p>
            <p className="muted">会话到期：{status.session_expires_at || '—'}</p>
            <form onSubmit={changePassword} className="form">
              <label>
                当前密码
                <input name="current_password" type="password" autoComplete="current-password" required />
              </label>
              <label>
                新密码（12-72 字节）
                <input name="new_password" type="password" autoComplete="new-password" required minLength={12} />
              </label>
              <button disabled={busy}>修改密码</button>
            </form>
            <button className="ghost" onClick={logout} disabled={busy}>
              退出登录
            </button>
          </>
        )}

        {health && (
          <dl>
            <div>
              <dt>服务</dt>
              <dd>
                {health.status}（{health.version}）
              </dd>
            </div>
            <div>
              <dt>数据库</dt>
              <dd>
                {health.database.status} / schema v{health.database.schema_version}
              </dd>
            </div>
            <div>
              <dt>/data</dt>
              <dd>{health.data.writable ? '可写' : '不可写'}</dd>
            </div>
            <div>
              <dt>进程</dt>
              <dd>
                uid {health.process.uid} / gid {health.process.gid}
                {health.process.is_root ? '（root）' : '（非 root）'}
              </dd>
            </div>
          </dl>
        )}
      </section>
    </main>
  )
}
