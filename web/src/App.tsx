import { useCallback, useEffect, useMemo, useRef, useState, useSyncExternalStore } from 'react'
import {
  APIError,
  createBookmark,
  deleteBookmark,
  loadAuth,
  loadBookmarks,
  loadLayout,
  loadNetworkProbeTargets,
  refreshFavicon,
  saveLayout,
  updateBookmark,
  uploadBookmarkIcon,
} from './api'
import type { AuthStatus, AutoNetworkStatus, Bookmark, BookmarkInput, CardSize, LayoutDevice, LayoutResponse, NetworkMode, Role, ThemeMode } from './types'
import { applyTheme, persistTheme, readTheme, watchSystemTheme } from './theme'
import {
  autoStatusText,
  bookmarkAddressMode,
  bookmarkAddressShortLabel,
  isNetworkMode,
  networkEntryState,
  networkModeLabel,
  networkModeStorageKey,
  persistNetworkMode,
  readNetworkMode,
} from './network'
import { AUTO_CACHE_TTL_MS, createAutoDetector } from './autodetect'
import { BookmarkCard } from './components/BookmarkCard'
import { AccountSheet } from './components/AccountSheet'
import { ThemeSwitcher } from './components/ThemeSwitcher'
import { Toast, type ToastMessage, type ToastTone } from './components/Toast'
import { BookmarkDrawer } from './components/BookmarkDrawer'
import { ConfirmDialog, type ConfirmRequest } from './components/ConfirmDialog'
import { EditToolbar } from './components/EditToolbar'
import { DockerSheet } from './components/DockerSheet'
import { NetworkSheet } from './components/NetworkSheet'
import { SpotlightSearch } from './components/SpotlightSearch'
import {
  appendEntry,
  bookmarksByLayout,
  buildLayouts,
  findEntry,
  layoutPayload,
  moveEntry,
  removeEntry,
  resizeEntry,
  toDraft,
  type DraftBookmark,
  type Layouts,
} from './editor'
import {
  BookmarkIcon,
  BrandMark,
  ChevronLeftIcon,
  ChevronRightIcon,
  FolderIcon,
  HomeIcon,
  NetworkIcon,
  PencilIcon,
  PlusIcon,
  RefreshIcon,
  SearchIcon,
  ServerIcon,
  UserIcon,
  WifiOffIcon,
} from './components/Icons'

type LoadState = 'loading' | 'ready' | 'error'
type Snapshot = { bookmarks: DraftBookmark[]; layouts: Layouts; deletedIDs: number[] }

const navItems = [
  { id: 'home', label: '首页', icon: HomeIcon },
  { id: 'bookmarks', label: '书签', icon: BookmarkIcon },
  { id: 'services', label: '服务', icon: ServerIcon },
  { id: 'search', label: '搜索', icon: SearchIcon },
] as const

const mobileNavItems = [...navItems, { id: 'account', label: '我的', icon: UserIcon }] as const

const emptyLayouts: Layouts = { desktop: [], mobile: [] }

function roleName(role: Role): string {
  return role === 'admin' ? '管理员' : '访客'
}

function avatarText(auth: AuthStatus | null, role: Role): string {
  if (auth?.authenticated && auth.username) return Array.from(auth.username)[0].toUpperCase()
  return role === 'admin' ? 'A' : '访'
}

function initialDevice(): LayoutDevice {
  if (typeof window === 'undefined') return 'desktop'
  return window.matchMedia?.('(max-width: 900px)').matches ? 'mobile' : 'desktop'
}

function errorMessage(error: unknown): string {
  if (error instanceof APIError) {
    if (error.status === 401 || error.status === 403) return '管理会话已失效，请重新登录后再保存'
    return error.message || '请求失败'
  }
  return '网络异常，请稍后重试'
}

export default function App() {
  const [themeMode, setThemeMode] = useState<ThemeMode>(readTheme)
  const [auth, setAuth] = useState<AuthStatus | null>(null)
  const [bookmarks, setBookmarks] = useState<Bookmark[]>([])
  const [role, setRole] = useState<Role>('guest')
  const [serverLayout, setServerLayout] = useState<LayoutResponse>({ desktop: [], mobile: [] })
  const [loadState, setLoadState] = useState<LoadState>('loading')
  const [reloadKey, setReloadKey] = useState(0)
  const [accountOpen, setAccountOpen] = useState(false)
  const [searchOpen, setSearchOpen] = useState(false)
  const [networkOpen, setNetworkOpen] = useState(false)
  const [dockerOpen, setDockerOpen] = useState(false)
  const [networkMode, setNetworkMode] = useState<NetworkMode>(readNetworkMode)
  // P4.3 自动判断只在当前浏览器执行；首页渲染与书签加载不等待它。
  const autoDetector = useMemo(() => createAutoDetector({ loadTargets: loadNetworkProbeTargets }), [])
  const auto = useSyncExternalStore(autoDetector.subscribe, autoDetector.getSnapshot, autoDetector.getSnapshot)
  const autoProbing = auto.phase === 'probing'
  const autoStatus: AutoNetworkStatus | null = auto.result?.status ?? null
  const networkEntry = networkEntryState(networkMode, autoStatus, autoProbing)
  const bookmarkAddress = bookmarkAddressMode(networkMode, autoStatus, autoProbing)
  const [collapsed, setCollapsed] = useState(() => {
    try {
      return localStorage.getItem('localhub.sidebar') === 'collapsed'
    } catch {
      return false
    }
  })
  const [toast, setToast] = useState<ToastMessage | null>(null)
  const toastCounter = useRef(0)

  // P3.2 编辑态。只有管理员进入后才会持有这些状态。
  const [editing, setEditing] = useState(false)
  const [preview, setPreview] = useState(false)
  const [drafts, setDrafts] = useState<DraftBookmark[]>([])
  const [layouts, setLayouts] = useState<Layouts>(emptyLayouts)
  const [deletedIDs, setDeletedIDs] = useState<number[]>([])
  const [device, setDevice] = useState<LayoutDevice>(initialDevice)
  const [history, setHistory] = useState<Snapshot[]>([])
  const [historyIndex, setHistoryIndex] = useState(0)
  const [drawer, setDrawer] = useState<{ open: boolean; initial: DraftBookmark | null }>({ open: false, initial: null })
  const [confirm, setConfirm] = useState<ConfirmRequest | null>(null)
  const [saving, setSaving] = useState(false)
  const [dragKey, setDragKey] = useState<string | null>(null)
  const [dragOverKey, setDragOverKey] = useState<string | null>(null)

  const historyRef = useRef<Snapshot[]>([])
  const historyIndexRef = useRef(0)
  const dragOverRef = useRef<string | null>(null)
  const createdIDsRef = useRef<Map<string, number>>(new Map())

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

  // 同一浏览器的标签页共享本机模式；不跨浏览器、不写服务端。
  useEffect(() => {
    const onStorage = (event: StorageEvent) => {
      if (event.key !== networkModeStorageKey) return
      setNetworkMode(isNetworkMode(event.newValue) ? event.newValue : 'auto')
    }
    window.addEventListener('storage', onStorage)
    return () => window.removeEventListener('storage', onStorage)
  }, [])

  // AUTO 打开、切回 AUTO、页面回到前台、网络变化与缓存过期都会重新判断。
  useEffect(() => {
    if (networkMode !== 'auto') return
    void autoDetector.run(false)
  }, [networkMode, autoDetector])

  useEffect(() => {
    if (networkMode !== 'auto') return
    const onVisibility = () => {
      if (document.visibilityState === 'visible') void autoDetector.run(false)
    }
    const onOnline = () => void autoDetector.run(true)
    const onOffline = () => autoDetector.invalidate('offline')
    const onConnectionChange = () => void autoDetector.run(false)
    const connection = (navigator as Navigator & { connection?: EventTarget }).connection
    document.addEventListener('visibilitychange', onVisibility)
    window.addEventListener('online', onOnline)
    window.addEventListener('offline', onOffline)
    connection?.addEventListener?.('change', onConnectionChange)
    return () => {
      document.removeEventListener('visibilitychange', onVisibility)
      window.removeEventListener('online', onOnline)
      window.removeEventListener('offline', onOffline)
      connection?.removeEventListener?.('change', onConnectionChange)
    }
  }, [networkMode, autoDetector])

  // 结果过期后只在前台补判；后台恢复由 visibilitychange 负责。
  useEffect(() => {
    if (networkMode !== 'auto' || auto.phase !== 'checked' || !auto.result) return
    const timer = window.setTimeout(() => {
      if (document.visibilityState === 'visible') void autoDetector.run(false)
    }, AUTO_CACHE_TTL_MS)
    return () => window.clearTimeout(timer)
  }, [networkMode, auto.phase, auto.result, autoDetector])

  useEffect(() => {
    if (!toast) return
    const timer = window.setTimeout(() => setToast(null), 3200)
    return () => window.clearTimeout(timer)
  }, [toast])

  useEffect(() => {
    const controller = new AbortController()
    setLoadState('loading')
    Promise.all([loadAuth(controller.signal), loadBookmarks(controller.signal), loadLayout(controller.signal)])
      .then(([status, list, layout]) => {
        setAuth(status)
        setRole(list.role)
        setBookmarks(list.items)
        setServerLayout(layout)
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

  // 非编辑态跟随屏幕断点切换布局；编辑态保持当前设备，避免拖动中被重置。
  useEffect(() => {
    const onResize = () => {
      if (!editing) setDevice(initialDevice())
    }
    window.addEventListener('resize', onResize)
    return () => window.removeEventListener('resize', onResize)
  }, [editing])

  const currentRole: Role = auth?.role ?? role
  const isAdmin = currentRole === 'admin' && Boolean(auth?.authenticated)

  useEffect(() => {
    if (editing && !isAdmin) {
      setEditing(false)
      setPreview(false)
      setDrawer({ open: false, initial: null })
      setDrafts([])
      setLayouts(emptyLayouts)
    }
  }, [editing, isAdmin])

  const changeTheme = useCallback((mode: ThemeMode) => setThemeMode(mode), [])

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

  const openAccount = useCallback(() => {
    setSearchOpen(false)
    setNetworkOpen(false)
    setAccountOpen(true)
  }, [])
  const closeAccount = useCallback(() => setAccountOpen(false), [])
  const openSearch = useCallback(() => {
    setAccountOpen(false)
    setNetworkOpen(false)
    setSearchOpen(true)
  }, [])
  const closeSearch = useCallback(() => setSearchOpen(false), [])
  const openNetwork = useCallback(() => {
    setAccountOpen(false)
    setSearchOpen(false)
    setNetworkOpen(true)
  }, [])
  const closeNetwork = useCallback(() => setNetworkOpen(false), [])
  const openDocker = useCallback(() => setDockerOpen(true), [])
  const closeDocker = useCallback(() => setDockerOpen(false), [])
  const changeNetworkMode = useCallback((next: NetworkMode) => {
    setNetworkMode(next)
    return persistNetworkMode(next)
  }, [])
  const recheckNetwork = useCallback(async () => {
    await autoDetector.run(true)
    const result = autoDetector.current()
    if (!result) return
    notify(`网络判断：${autoStatusText(result.status)}`, result.status === 'lan' ? 'success' : 'info')
  }, [autoDetector, notify])

  function openNavigation(label: string) {
    notify(`「${label}」将在后续批次开放`)
  }

  const viewDrafts = useMemo(() => bookmarks.map(toDraft), [bookmarks])
  const viewLayouts = useMemo(() => buildLayouts(viewDrafts, serverLayout), [viewDrafts, serverLayout])
  const visibleDrafts = useMemo(
    () => bookmarksByLayout(editing ? drafts : viewDrafts, (editing ? layouts : viewLayouts)[device]),
    [editing, drafts, viewDrafts, layouts, viewLayouts, device],
  )
  const currentEntries = (editing ? layouts : viewLayouts)[device]
  const isDirty = editing && historyIndex > 0
  const canUndo = editing && historyIndex > 0
  const canRedo = editing && historyIndex < history.length - 1

  function pushHistory(snapshot: Snapshot) {
    const next = historyRef.current.slice(0, historyIndexRef.current + 1)
    next.push(snapshot)
    historyRef.current = next
    historyIndexRef.current = next.length - 1
    setHistory(next)
    setHistoryIndex(next.length - 1)
    setDrafts(snapshot.bookmarks)
    setLayouts(snapshot.layouts)
    setDeletedIDs(snapshot.deletedIDs)
  }

  function resetEditor(baseBookmarks: Bookmark[], baseLayout: LayoutResponse) {
    const baseDrafts = baseBookmarks.map(toDraft)
    const baseLayouts = buildLayouts(baseDrafts, baseLayout)
    const snapshot: Snapshot = { bookmarks: baseDrafts, layouts: baseLayouts, deletedIDs: [] }
    historyRef.current = [snapshot]
    historyIndexRef.current = 0
    createdIDsRef.current = new Map()
    setHistory([snapshot])
    setHistoryIndex(0)
    setDrafts(baseDrafts)
    setLayouts(baseLayouts)
    setDeletedIDs([])
    setEditing(true)
    setPreview(false)
    setDevice(initialDevice())
  }

  const beginEditing = useCallback(() => {
    if (!isAdmin) return
    setSearchOpen(false)
    setNetworkOpen(false)
    resetEditor(bookmarks, serverLayout)
  }, [isAdmin, bookmarks, serverLayout])

  const leaveEditor = useCallback(() => {
    setEditing(false)
    setPreview(false)
    setDrawer({ open: false, initial: null })
    setDrafts([])
    setLayouts(emptyLayouts)
    setDeletedIDs([])
    setHistory([])
    setHistoryIndex(0)
    setDragKey(null)
    setDragOverKey(null)
  }, [])

  const requestCancel = useCallback(() => {
    if (!isDirty) {
      leaveEditor()
      return
    }
    setConfirm({
      title: '放弃未保存修改？',
      message: '当前新增、编辑、删除、排序和尺寸修改尚未保存，离开后会全部丢失。',
      confirmLabel: '放弃修改',
      tone: 'danger',
      onConfirm: leaveEditor,
    })
  }, [isDirty, leaveEditor])

  useEffect(() => {
    if (!editing || !isDirty) return
    const handler = (event: BeforeUnloadEvent) => {
      event.preventDefault()
      event.returnValue = ''
    }
    window.addEventListener('beforeunload', handler)
    return () => window.removeEventListener('beforeunload', handler)
  }, [editing, isDirty])

  function undo() {
    if (!canUndo) return
    const nextIndex = historyIndexRef.current - 1
    const snapshot = historyRef.current[nextIndex]
    if (!snapshot) return
    historyIndexRef.current = nextIndex
    setHistoryIndex(nextIndex)
    setDrafts(snapshot.bookmarks)
    setLayouts(snapshot.layouts)
    setDeletedIDs(snapshot.deletedIDs)
    setDrawer({ open: false, initial: null })
  }

  function redo() {
    if (!canRedo) return
    const nextIndex = historyIndexRef.current + 1
    const snapshot = historyRef.current[nextIndex]
    if (!snapshot) return
    historyIndexRef.current = nextIndex
    setHistoryIndex(nextIndex)
    setDrafts(snapshot.bookmarks)
    setLayouts(snapshot.layouts)
    setDeletedIDs(snapshot.deletedIDs)
    setDrawer({ open: false, initial: null })
  }

  function submitDraft(next: DraftBookmark) {
    const exists = drafts.some((bookmark) => bookmark.key === next.key)
    const nextBookmarks = exists ? drafts.map((bookmark) => (bookmark.key === next.key ? next : bookmark)) : [...drafts, next]
    let nextLayouts = layouts
    if (!exists) {
      nextLayouts = {
        desktop: appendEntry(layouts.desktop, next.key, 'medium'),
        mobile: appendEntry(layouts.mobile, next.key, 'medium'),
      }
    }
    setDrawer({ open: false, initial: null })
    pushHistory({ bookmarks: nextBookmarks, layouts: nextLayouts, deletedIDs })
  }

  function requestDelete(bookmark: DraftBookmark) {
    setConfirm({
      title: '删除书签？',
      message: `确认删除「${bookmark.name}」？该操作会在保存后生效，删除前还可以撤销。`,
      confirmLabel: '删除',
      tone: 'danger',
      onConfirm: () => {
        const serverID = bookmark.serverId ?? createdIDsRef.current.get(bookmark.key) ?? (bookmark.isNew ? undefined : bookmark.id)
        const nextDeleted = serverID && serverID > 0 && !deletedIDs.includes(serverID) ? [...deletedIDs, serverID] : deletedIDs
        const nextBookmarks = drafts.filter((item) => item.key !== bookmark.key)
        const nextLayouts = {
          desktop: removeEntry(layouts.desktop, bookmark.key),
          mobile: removeEntry(layouts.mobile, bookmark.key),
        }
        setDrawer({ open: false, initial: null })
        pushHistory({ bookmarks: nextBookmarks, layouts: nextLayouts, deletedIDs: nextDeleted })
      },
    })
  }

  function changeSize(key: string, size: CardSize) {
    const nextEntries = resizeEntry(layouts[device], key, size)
    if (nextEntries === layouts[device]) return
    pushHistory({ bookmarks: drafts, layouts: { ...layouts, [device]: nextEntries }, deletedIDs })
  }

  const finishDrag = useCallback(() => {
    const source = dragKey
    const target = dragOverRef.current
    if (source && target && source !== target) {
      const nextEntries = moveEntry(layouts[device], source, target)
      if (nextEntries !== layouts[device]) {
        pushHistory({ bookmarks: drafts, layouts: { ...layouts, [device]: nextEntries }, deletedIDs })
      }
    }
    setDragKey(null)
    dragOverRef.current = null
    setDragOverKey(null)
  }, [dragKey, layouts, drafts, deletedIDs, device])

  useEffect(() => {
    if (!dragKey) return
    const move = (event: PointerEvent) => {
      const element = document.elementFromPoint(event.clientX, event.clientY) as HTMLElement | null
      const card = element?.closest('[data-key]') as HTMLElement | null
      const key = card?.dataset.key ?? null
      dragOverRef.current = key
      setDragOverKey(key)
    }
    const up = () => finishDrag()
    window.addEventListener('pointermove', move, { passive: true })
    window.addEventListener('pointerup', up, { once: true })
    window.addEventListener('pointercancel', up, { once: true })
    return () => {
      window.removeEventListener('pointermove', move)
      window.removeEventListener('pointerup', up)
      window.removeEventListener('pointercancel', up)
    }
  }, [dragKey, finishDrag])

  function startDrag(key: string, event: React.PointerEvent<HTMLButtonElement>) {
    if (saving) return
    event.preventDefault()
    event.stopPropagation()
    dragOverRef.current = null
    setDragOverKey(null)
    setDragKey(key)
  }

  function inputFor(draft: DraftBookmark): BookmarkInput {
    const input: BookmarkInput = {
      name: draft.name,
      lan_url: draft.lan_url,
      wan_url: draft.wan_url,
      description: draft.description,
      enabled: draft.enabled,
      visibility: draft.visibility,
      open_mode: draft.open_mode,
      folder_path: draft.folder_path,
    }
    // 图标文件在最终保存时通过专用上传接口提交；表单阶段只保留本地预览。
    // 已存在的 file 图标在未替换时也不重新发送，后端 PATCH 会保持原值。
    if (!draft.iconBlob && draft.icon_type !== 'file') {
      input.icon_type = draft.icon_type
      input.icon_value = draft.icon_value
    }
    return input
  }

  async function persistEditor() {
    if (saving) return
    setSaving(true)
    try {
      const savedBookmarks: DraftBookmark[] = []
      for (const draft of drafts) {
        const knownID = draft.serverId ?? createdIDsRef.current.get(draft.key) ?? (!draft.isNew ? draft.id : undefined)
        const input = inputFor(draft)
        if (knownID && knownID > 0) {
          await updateBookmark(knownID, input)
          savedBookmarks.push({ ...draft, serverId: knownID, isNew: false })
        } else {
          const created = await createBookmark(input)
          createdIDsRef.current.set(draft.key, created.id)
          savedBookmarks.push({ ...draft, serverId: created.id, id: created.id, isNew: false })
        }
      }

      for (const draft of savedBookmarks) {
        const id = draft.serverId
        if (!id) continue
        if (draft.iconBlob) {
          await uploadBookmarkIcon(id, draft.iconBlob)
        }
        if (draft.icon_type === 'favicon' && !draft.iconBlob) {
          // 新建/编辑已触发后端自动抓取；这里显式兜底一次，失败不阻断保存。
          try {
            await refreshFavicon(id)
          } catch {
            /* favicon 抓取失败不阻塞布局保存，前端显示首字母占位 */
          }
        }
      }

      for (const id of deletedIDs) {
        try {
          await deleteBookmark(id)
        } catch (error) {
          if (!(error instanceof APIError) || error.status !== 404) throw error
        }
      }

      const patchedDrafts = savedBookmarks
      const payload = layoutPayload(layouts, patchedDrafts)
      await saveLayout(payload)

      try {
        const [list, remote] = await Promise.all([loadBookmarks(), loadLayout()])
        setBookmarks(list.items)
        setServerLayout(remote)
        setRole(list.role)
        setLoadState('ready')
        setSaving(false)
        leaveEditor()
        notify('首页书签与布局已保存', 'success')
      } catch (reloadError) {
        // 服务端保存已经成功，不能误报为“保存失败”；明确提示刷新失败并触发重新加载。
        setSaving(false)
        leaveEditor()
        setReloadKey((value) => value + 1)
        notify(`布局已保存，但刷新首页失败：${errorMessage(reloadError)}`, 'error')
      }
    } catch (error) {
      setSaving(false)
      notify(`保存失败：${errorMessage(error)}。当前编辑内容已保留，可重试。`, 'error')
      if (error instanceof APIError && (error.status === 401 || error.status === 403)) {
        setReloadKey((value) => value + 1)
      }
    }
  }

  const currentSize = drawer.initial ? findEntry(layouts[device], drawer.initial.key)?.size : undefined

  return (
    <div className={`app-shell${editing ? ' is-editing' : ''}`}>
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
                onClick={() => {
                  if (item.id === 'search') openSearch()
                  else if (item.id === 'services') openDocker()
                  else if (!active) openNavigation(item.label)
                }}
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
          {!editing && (
            <button
              type="button"
              className="mobile-network-entry"
              onClick={openNetwork}
              aria-label={`网络模式 ${networkModeLabel(networkMode)}，当前 ${networkEntry.label}，书签使用${bookmarkAddressShortLabel(bookmarkAddress)}，打开网络设置`}
            >
              <NetworkIcon size={15} />
              <span className={`network-state-dot network-state-dot--${networkEntry.tone}`} aria-hidden="true" />
              <strong>{networkEntry.label}</strong>
            </button>
          )}
          <ThemeSwitcher mode={themeMode} onChange={changeTheme} compact />
          <button type="button" className="avatar avatar-button" onClick={openAccount} aria-label="打开账户面板">
            {avatarText(auth, currentRole)}
          </button>
        </div>
      </header>

      <main className="main-content">
        <section className="content-inner" aria-labelledby="page-title">
          <header className="page-heading">
            <div>
              <h1 id="page-title">书签</h1>
              {editing && <span className="heading-mode">{preview ? '预览模式' : '编辑模式'}</span>}
            </div>
            <div className="page-actions">
              {!editing && (
                <button
                  type="button"
                  className="network-entry"
                  onClick={openNetwork}
                  aria-label={`网络模式 ${networkModeLabel(networkMode)}，当前 ${networkEntry.label}，书签使用${bookmarkAddressShortLabel(bookmarkAddress)}，打开网络设置`}
                >
                  <NetworkIcon size={16} />
                  <span className="network-entry-label">网络</span>
                  <span className={`network-state-dot network-state-dot--${networkEntry.tone}`} aria-hidden="true" />
                  <strong className={`network-state-text network-state-text--${networkEntry.tone}`}>
                    {networkEntry.label}
                  </strong>
                  {networkEntry.preference === 'auto' && networkEntry.label !== 'AUTO' && (
                    <span className="network-entry-auto">AUTO</span>
                  )}
                </button>
              )}
              {loadState === 'ready' && !editing && (
                <button type="button" className="search-entry" onClick={openSearch} aria-label="搜索书签">
                  <SearchIcon size={16} />
                  搜索
                </button>
              )}
              {loadState === 'ready' && isAdmin && !editing && (
                <button type="button" className="edit-entry" onClick={beginEditing}>
                  <PencilIcon size={16} />
                  编辑首页
                </button>
              )}
              {loadState === 'error' && (
                <button type="button" className="icon-button" onClick={() => refresh()} aria-label="重新加载书签" title="重新加载">
                  <RefreshIcon size={18} />
                </button>
              )}
            </div>
          </header>

          {editing && (
            <EditToolbar
              device={device}
              onDeviceChange={setDevice}
              dirty={isDirty}
              preview={preview}
              canUndo={canUndo}
              canRedo={canRedo}
              saving={saving}
              onAdd={() => setDrawer({ open: true, initial: null })}
              onUndo={undo}
              onRedo={redo}
              onPreview={() => setPreview((value) => !value)}
              onCancel={requestCancel}
              onSave={persistEditor}
            />
          )}

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

          {loadState === 'ready' && visibleDrafts.length === 0 && (
            <div className="state-card">
              <span className="state-icon">
                <FolderIcon size={30} />
              </span>
              <h2>{editing ? '还没有书签' : currentRole === 'admin' ? '书签库还是空的' : '暂时没有公开书签'}</h2>
              <p>
                {editing
                  ? '新增书签后，它会以中号卡片加入桌面和移动端布局。'
                  : currentRole === 'admin'
                    ? '点击右上角「编辑首页」添加第一枚书签。'
                    : '管理员公开书签后，会显示在这里。'}
              </p>
              {editing && (
                <button type="button" className="primary-button" onClick={() => setDrawer({ open: true, initial: null })}>
                  <PlusIcon size={16} />
                  新增书签
                </button>
              )}
            </div>
          )}

          {loadState === 'ready' && visibleDrafts.length > 0 && (
            <div className={`bookmark-grid${editing && !preview ? ' is-editing-grid' : ''}`} aria-label={preview ? '首页预览' : '书签列表'}>
              {visibleDrafts.map((bookmark, index) => {
                const size = editing ? findEntry(currentEntries, bookmark.key)?.size ?? 'medium' : currentEntries.find((entry) => entry.key === bookmark.key)?.size ?? 'medium'
                return (
                  <BookmarkCard
                    bookmark={bookmark}
                    index={index}
                    size={size}
                    addressMode={bookmarkAddress}
                    key={bookmark.key}
                    editing={editing && !preview}
                    dragOver={dragOverKey === bookmark.key}
                    dataKey={bookmark.key}
                    onEdit={() => setDrawer({ open: true, initial: bookmark })}
                    onSizeChange={(nextSize) => changeSize(bookmark.key, nextSize)}
                    onDragStart={(event) => startDrag(bookmark.key, event)}
                  />
                )
              })}
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
                else if (item.id === 'search') openSearch()
                else if (item.id === 'services') openDocker()
                else if (!active) openNavigation(item.label)
              }}
            >
              <Icon size={22} />
              <span>{item.label}</span>
            </button>
          )
        })}
      </nav>

      {networkOpen && (
        <NetworkSheet
          mode={networkMode}
          isAdmin={isAdmin}
          autoPhase={auto.phase}
          autoResult={auto.result}
          onRecheck={recheckNetwork}
          onModeChange={changeNetworkMode}
          onClose={closeNetwork}
        />
      )}
      {dockerOpen && <DockerSheet isAdmin={isAdmin} onClose={closeDocker} />}
      {searchOpen && <SpotlightSearch bookmarks={bookmarks} addressMode={bookmarkAddress} onClose={closeSearch} />}
      {toast && <Toast toast={toast} onClose={() => setToast(null)} />}

      {accountOpen && <AccountSheet auth={auth} onClose={closeAccount} onRefresh={refresh} />}
      {drawer.open && (
        <BookmarkDrawer
          open={drawer.open}
          initial={drawer.initial}
          currentSize={currentSize}
          onClose={() => setDrawer({ open: false, initial: null })}
          onSubmit={submitDraft}
          onRequestDelete={requestDelete}
        />
      )}
      {confirm && <ConfirmDialog request={confirm} onCancel={() => setConfirm(null)} />}
    </div>
  )
}
