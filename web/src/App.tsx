import { useCallback, useEffect, useRef, useState } from 'react'
import { APIError, loadAuth, loadBookmarks } from './api'
import type { AuthStatus, Bookmark, Role, ThemeMode } from './types'
import { applyTheme, persistTheme, readTheme, watchSystemTheme } from './theme'
import { BookmarkCard } from './components/BookmarkCard'
import { AccountSheet } from './components/AccountSheet'
import { ThemeSwitcher } from './components/ThemeSwitcher'
import { Toast, type ToastMessage, type ToastTone } from './components/Toast'
import {
  BookmarkIcon,
  BrandMark,
  ChevronLeftIcon,
  ChevronRightIcon,
  FolderIcon,
  HomeIcon,
  RefreshIcon,
  SearchIcon,
  ServerIcon,
  UserIcon,
  WifiOffIcon,
} from './components/Icons'

type LoadState = 'loading' | 'ready' | 'error'

const navItems = [
  { id: 'home', label: '首页', icon: HomeIcon },
  { id: 'bookmarks', label: '书签', icon: BookmarkIcon },
  { id: 'services', label: '服务', icon: ServerIcon },
  { id: 'search', label: '搜索', icon: SearchIcon },
] as const

const mobileNavItems = [
  ...navItems,
  { id: 'account', label: '我的', icon: UserIcon },
] as const

function roleName(role: Role): string {
  return role === 'admin' ? '管理员' : '访客'
}

function avatarText(auth: AuthStatus | null, role: Role): string {
  if (auth?.authenticated && auth.username) return Array.from(auth.username)[0].toUpperCase()
  return role === 'admin' ? 'A' : '访'
}

export default function App() {
  const [themeMode, setThemeMode] = useState<ThemeMode>(readTheme)
  const [auth, setAuth] = useState<AuthStatus | null>(null)
  const [bookmarks, setBookmarks] = useState<Bookmark[]>([])
  const [role, setRole] = useState<Role>('guest')
  const [loadState, setLoadState] = useState<LoadState>('loading')
  const [reloadKey, setReloadKey] = useState(0)
  const [accountOpen, setAccountOpen] = useState(false)
  const [collapsed, setCollapsed] = useState(() => {
    try {
      return localStorage.getItem('localhub.sidebar') === 'collapsed'
    } catch {
      return false
    }
  })
  const [toast, setToast] = useState<ToastMessage | null>(null)
  const toastCounter = useRef(0)

  const notify = useCallback((text: string, tone: ToastTone = 'info') => {
    toastCounter.current += 1
    setToast({ id: toastCounter.current, text, tone })
  }, [])

  useEffect(() => {
    persistTheme(themeMode)
    applyTheme(themeMode)
    if (themeMode !== 'auto') return
    return watchSystemTheme(() => applyTheme('auto'))
  }, [themeMode])

  useEffect(() => {
    if (!toast) return
    const timer = window.setTimeout(() => setToast(null), 3200)
    return () => window.clearTimeout(timer)
  }, [toast])

  useEffect(() => {
    const controller = new AbortController()
    setLoadState('loading')
    Promise.all([loadAuth(controller.signal), loadBookmarks(controller.signal)])
      .then(([status, list]) => {
        setAuth(status)
        setRole(list.role)
        setBookmarks(list.items)
        setLoadState('ready')
      })
      .catch((error: unknown) => {
        if (controller.signal.aborted) return
        setLoadState('error')
        if (error instanceof APIError) {
          notify(error.status === 503 ? '服务暂时不可用' : '书签加载失败', 'error')
        } else {
          notify('无法连接 LocalHub', 'error')
        }
      })
    return () => controller.abort()
  }, [reloadKey, notify])

  const changeTheme = useCallback((mode: ThemeMode) => {
    setThemeMode(mode)
  }, [])

  const toggleSidebar = useCallback(() => {
    setCollapsed((value) => {
      const next = !value
      try {
        localStorage.setItem('localhub.sidebar', next ? 'collapsed' : 'expanded')
      } catch {
        /* 存储失败不影响本次交互 */
      }
      return next
    })
  }, [])

  const refresh = useCallback(
    (message?: string) => {
      if (message) notify(message, 'success')
      setReloadKey((value) => value + 1)
    },
    [notify],
  )

  const openAccount = useCallback(() => setAccountOpen(true), [])
  const closeAccount = useCallback(() => setAccountOpen(false), [])

  function openNavigation(label: string) {
    notify(`「${label}」将在后续批次开放`)
  }

  const currentRole: Role = auth?.role ?? role

  return (
    <div className="app-shell">
      <div className="ambient ambient-one" aria-hidden="true" />
      <div className="ambient ambient-two" aria-hidden="true" />

      <aside className={`sidebar${collapsed ? ' is-collapsed' : ''}`} aria-label="主导航">
        <div className="sidebar-head">
          <div className="brand">
            <BrandMark size={36} />
            <div className="brand-copy">
              <strong>LocalHub</strong>
              <span>Home</span>
            </div>
          </div>
          <button
            type="button"
            className="icon-button collapse-button"
            onClick={toggleSidebar}
            aria-label={collapsed ? '展开侧边栏' : '收起侧边栏'}
            aria-expanded={!collapsed}
            title={collapsed ? '展开侧边栏' : '收起侧边栏'}
          >
            {collapsed ? <ChevronRightIcon size={17} /> : <ChevronLeftIcon size={17} />}
          </button>
        </div>

        <nav className="sidebar-nav">
          {navItems.map((item) => {
            const Icon = item.icon
            const active = item.id === 'home'
            return (
              <button
                key={item.id}
                type="button"
                className={`nav-item${active ? ' is-active' : ''}`}
                aria-current={active ? 'page' : undefined}
                title={collapsed ? item.label : undefined}
                onClick={() => !active && openNavigation(item.label)}
              >
                <Icon size={19} />
                <span>{item.label}</span>
              </button>
            )
          })}
        </nav>

        <div className="sidebar-foot">
          <button type="button" className="account-row" onClick={openAccount} aria-label="打开账户面板">
            <span className="avatar" aria-hidden="true">
              {avatarText(auth, currentRole)}
            </span>
            <span className="account-copy">
              <strong>{auth?.authenticated ? auth.username : '访客'}</strong>
              <span>{roleName(currentRole)}</span>
            </span>
            <ChevronRightIcon size={15} />
          </button>

          <div className="foot-theme">
            <span className="foot-label">外观</span>
            <ThemeSwitcher mode={themeMode} onChange={changeTheme} compact={collapsed} />
          </div>
        </div>
      </aside>

      <header className="mobile-topbar">
        <div className="mobile-brand">
          <BrandMark size={32} />
          <div>
            <strong>LocalHub</strong>
            <span>Home</span>
          </div>
        </div>
        <div className="mobile-actions">
          <ThemeSwitcher mode={themeMode} onChange={changeTheme} compact />
          <button type="button" className="avatar avatar-button" onClick={openAccount} aria-label="打开账户面板">
            {avatarText(auth, currentRole)}
          </button>
        </div>
      </header>

      <main className="main-content">
        <section className="content-inner" aria-labelledby="page-title">
          <header className="page-heading">
            <h1 id="page-title">书签</h1>
            {loadState === 'error' && (
              <button type="button" className="icon-button" onClick={() => refresh()} aria-label="重新加载书签" title="重新加载">
                <RefreshIcon size={18} />
              </button>
            )}
          </header>

          {loadState === 'loading' && (
            <div className="bookmark-grid skeleton-grid" aria-busy="true" aria-label="正在加载书签">
              {Array.from({ length: 8 }, (_, index) => (
                <div className="bookmark-card skeleton-card" data-size={index === 0 ? 'large' : 'medium'} key={index}>
                  <span className="skeleton-icon" />
                  <span className="skeleton-name" />
                </div>
              ))}
            </div>
          )}

          {loadState === 'error' && (
            <div className="state-card" role="alert">
              <span className="state-icon">
                <WifiOffIcon size={28} />
              </span>
              <h2>无法加载书签</h2>
              <p>连接服务失败，或当前会话已失效。</p>
              <button type="button" className="primary-button" onClick={() => refresh()}>
                <RefreshIcon size={17} />
                重新加载
              </button>
            </div>
          )}

          {loadState === 'ready' && bookmarks.length === 0 && (
            <div className="state-card">
              <span className="state-icon">
                <FolderIcon size={30} />
              </span>
              <h2>{currentRole === 'admin' ? '书签库还是空的' : '暂时没有公开书签'}</h2>
              <p>{currentRole === 'admin' ? '添加书签后，它们会按排序出现在这里。' : '管理员公开书签后，会显示在这里。'}</p>
            </div>
          )}

          {loadState === 'ready' && bookmarks.length > 0 && (
            <div className="bookmark-grid" aria-label="书签列表">
              {bookmarks.map((bookmark, index) => (
                <BookmarkCard bookmark={bookmark} index={index} key={bookmark.id} />
              ))}
            </div>
          )}
        </section>
      </main>

      <nav className="mobile-nav" aria-label="移动端导航">
        {mobileNavItems.map((item) => {
          const Icon = item.icon
          const active = item.id === 'home'
          return (
            <button
              key={item.id}
              type="button"
              className={`mobile-nav-item${active ? ' is-active' : ''}`}
              aria-current={active ? 'page' : undefined}
              onClick={() => {
                if (item.id === 'account') openAccount()
                else if (!active) openNavigation(item.label)
              }}
            >
              <Icon size={22} />
              <span>{item.label}</span>
            </button>
          )
        })}
      </nav>

      {toast && <Toast toast={toast} onClose={() => setToast(null)} />}

      {accountOpen && <AccountSheet auth={auth} onClose={closeAccount} onRefresh={refresh} />}
    </div>
  )
}
