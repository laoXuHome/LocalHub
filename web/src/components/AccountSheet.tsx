import { useEffect, useRef, useState } from 'react'
import { changePassword, login, logout, APIError } from '../api'
import type { AuthStatus, Role } from '../types'
import { CloseIcon, KeyIcon, LogoutIcon, ShieldIcon, UserIcon } from './Icons'

function errorMessage(error: unknown): string {
  if (error instanceof APIError) {
    if (error.status === 429) return '尝试次数过多，请稍后再试'
    if (error.status === 401) return '用户名或密码不正确'
    return error.message
  }
  return '网络异常，请稍后再试'
}

function sessionLabel(value?: string): string {
  if (!value) return ''
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return ''
  return date.toLocaleString('zh-CN', { month: 'numeric', day: 'numeric', hour: '2-digit', minute: '2-digit' })
}

export function AccountSheet({
  auth,
  onClose,
  onRefresh,
}: {
  auth: AuthStatus | null
  onClose: () => void
  onRefresh: (message: string) => void
}) {
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')
  const [role, setRole] = useState<Role>('guest')
  const firstField = useRef<HTMLInputElement>(null)
  const panel = useRef<HTMLDivElement>(null)

  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      if (event.key === 'Escape') onClose()
    }
    document.addEventListener('keydown', onKey)
    const previousOverflow = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    const timer = window.setTimeout(() => firstField.current?.focus(), 80)
    return () => {
      document.removeEventListener('keydown', onKey)
      document.body.style.overflow = previousOverflow
      window.clearTimeout(timer)
    }
  }, [onClose])

  useEffect(() => {
    if (!auth?.authenticated) return
    setRole(auth.role)
  }, [auth])

  async function submitLogin(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const data = new FormData(event.currentTarget)
    setBusy(true)
    setError('')
    try {
      await login(String(data.get('username') || ''), String(data.get('password') || ''))
      onRefresh('已登录，私有书签已显示')
      onClose()
    } catch (err) {
      setError(errorMessage(err))
    } finally {
      setBusy(false)
    }
  }

  async function submitPassword(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const form = event.currentTarget
    const data = new FormData(form)
    setBusy(true)
    setError('')
    setNotice('')
    try {
      await changePassword(String(data.get('current_password') || ''), String(data.get('new_password') || ''))
      form.reset()
      setNotice('密码已修改，其他设备需重新登录')
      onRefresh('密码已修改')
    } catch (err) {
      setError(errorMessage(err))
    } finally {
      setBusy(false)
    }
  }

  async function doLogout() {
    setBusy(true)
    setError('')
    try {
      await logout()
      onRefresh('已退出登录，仅显示公开书签')
      onClose()
    } catch (err) {
      setError(errorMessage(err))
    } finally {
      setBusy(false)
    }
  }

  const authenticated = Boolean(auth?.authenticated)
  const needsSetup = Boolean(auth?.needs_setup)
  const session = sessionLabel(auth?.session_expires_at)

  return (
    <div className="sheet-layer" role="presentation" onMouseDown={(event) => event.target === event.currentTarget && onClose()}>
      <div className="account-sheet" role="dialog" aria-modal="true" aria-labelledby="account-title" ref={panel}>
        <div className="sheet-handle" aria-hidden="true" />
        <header className="sheet-head">
          <div>
            <p className="eyebrow">LOCALHUB ACCOUNT</p>
            <h2 id="account-title">{authenticated ? '账户' : '登录'}</h2>
          </div>
          <button type="button" className="sheet-close" onClick={onClose} aria-label="关闭账户面板">
            <CloseIcon size={18} />
          </button>
        </header>

        {authenticated ? (
          <>
            <div className="account-identity">
              <span className="account-avatar" aria-hidden="true">
                <UserIcon size={22} />
              </span>
              <div className="account-meta">
                <strong>{auth?.username || 'admin'}</strong>
                <span>{role === 'admin' ? '管理员 · 全部可见书签' : '访客 · 公开书签'}</span>
                {session && <small>会话有效至 {session}</small>}
              </div>
              <span className="role-chip">ADMIN</span>
            </div>

            <form className="account-form" onSubmit={submitPassword}>
              <div className="form-heading">
                <KeyIcon size={17} />
                <span>修改密码</span>
              </div>
              <label>
                当前密码
                <input name="current_password" type="password" autoComplete="current-password" required disabled={busy} />
              </label>
              <label>
                新密码 <small>12–72 字节</small>
                <input
                  name="new_password"
                  type="password"
                  autoComplete="new-password"
                  required
                  minLength={12}
                  disabled={busy}
                />
              </label>
              <button type="submit" className="primary-button" disabled={busy}>
                {busy ? '处理中…' : '保存新密码'}
              </button>
            </form>

            <button type="button" className="quiet-button danger-text" onClick={doLogout} disabled={busy}>
              <LogoutIcon size={17} />
              退出登录
            </button>
          </>
        ) : (
          <>
            <div className="guest-note">
              <ShieldIcon size={19} />
              <p>访客可以浏览公开书签。登录管理员后可查看登录后可见与私有书签。</p>
            </div>

            {needsSetup ? (
              <div className="setup-note">
                <strong>管理员尚未初始化</strong>
                <p>请设置 `LOCALHUB_ADMIN_PASSWORD`（12–72 字节）后重启容器，再返回此面板登录。</p>
              </div>
            ) : (
              <form className="account-form" onSubmit={submitLogin}>
                <label>
                  用户名
                  <input ref={firstField} name="username" defaultValue="admin" autoComplete="username" required disabled={busy} />
                </label>
                <label>
                  密码
                  <input name="password" type="password" autoComplete="current-password" required disabled={busy} />
                </label>
                <button type="submit" className="primary-button" disabled={busy}>
                  {busy ? '正在登录…' : '登录'}
                </button>
              </form>
            )}
          </>
        )}

        {error && <p className="form-message form-message--error">{error}</p>}
        {notice && <p className="form-message form-message--ok">{notice}</p>}
      </div>
    </div>
  )
}
