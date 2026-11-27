import { useEffect, useState } from 'react'

type Health = {
  status: string
  version: string
  commit: string
  build_time: string
  uptime_seconds: number
  tz: string
  database: { status: string; path: string; schema_version: number; error?: string }
  data: { path: string; allowed_root: string; writable: boolean; last_check: string; error?: string }
  process: { role: string; pid: number; uid: number; gid: number; is_root: boolean }
}

export default function App() {
  const [health, setHealth] = useState<Health | null>(null)
  const [error, setError] = useState('')

  useEffect(() => {
    let alive = true
    const load = () => {
      fetch('/health')
        .then((r) => r.json())
        .then((h: Health) => {
          if (alive) {
            setHealth(h)
            setError('')
          }
        })
        .catch(() => alive && setError('无法读取 /health'))
    }
    load()
    const timer = setInterval(load, 15000)
    return () => {
      alive = false
      clearInterval(timer)
    }
  }, [])

  const ok = health?.status === 'ok'

  return (
    <main className="page">
      <section className="card">
        <h1>LocalHub · P0 基础建设</h1>
        <p className="muted">本页面只用于确认容器已启动，不含任何产品功能。</p>

        {error && <p className="bad">{error}</p>}
        {!health && !error && <p className="muted">正在读取 /health…</p>}

        {health && (
          <dl>
            <div>
              <dt>状态</dt>
              <dd>
                <span className={ok ? 'dot ok' : 'dot bad'} />
                {health.status}
              </dd>
            </div>
            <div>
              <dt>版本</dt>
              <dd>
                {health.version} / {health.commit}
              </dd>
            </div>
            <div>
              <dt>数据库</dt>
              <dd>
                {health.database.status}（schema v{health.database.schema_version}）
              </dd>
            </div>
            <div>
              <dt>/data 可写</dt>
              <dd>{health.data.writable ? '是' : '否'}</dd>
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
