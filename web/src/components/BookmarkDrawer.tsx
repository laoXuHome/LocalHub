import { useEffect, useRef, useState } from 'react'
import type { BookmarkInput, CardSize, DockerContainersInfo } from '../types'
import type { DraftBookmark } from '../editor'
import { draftKey } from '../editor'
import { initialLetter, isHTTPURL } from '../lib'
import { loadDockerContainers } from '../api'
import { containerStatusLabel, containerStatusTone } from '../docker'
import { CloseIcon } from './Icons'

type IconChoice = 'none' | 'text' | 'url' | 'favicon' | 'file'

const iconChoices: { value: IconChoice; label: string }[] = [
  { value: 'none', label: '无' },
  { value: 'text', label: '文字' },
  { value: 'url', label: '图片链接' },
  { value: 'favicon', label: 'Favicon' },
  { value: 'file', label: '上传' },
]

function choiceFor(bookmark: DraftBookmark | null): IconChoice {
  if (!bookmark) return 'none'
  if (bookmark.icon_type === 'file') return 'file'
  if (bookmark.icon_type === 'favicon') return 'favicon'
  if (bookmark.icon_type === 'url') return 'url'
  if (bookmark.icon_type === 'text' || bookmark.icon_type === 'emoji') return 'text'
  return 'none'
}

function emptyDraft(): DraftBookmark {
  return {
    id: -1,
    key: draftKey(),
    name: '',
    lan_url: '',
    wan_url: '',
    description: '',
    icon_type: 'none',
    icon_value: '',
    has_icon: false,
    enabled: true,
    visibility: 'public',
    effective_visibility: 'public',
    open_mode: 'new_tab',
    docker_container: '',
    folder_path: [],
    sort_order: 0,
    isNew: true,
  }
}

export function BookmarkDrawer({
  open,
  initial,
  onClose,
  onSubmit,
  onRequestDelete,
  currentSize,
}: {
  open: boolean
  initial: DraftBookmark | null
  onClose: () => void
  onSubmit: (bookmark: DraftBookmark) => void
  onRequestDelete?: (bookmark: DraftBookmark) => void
  currentSize?: CardSize
}) {
  const [name, setName] = useState('')
  const [lanURL, setLanURL] = useState('')
  const [wanURL, setWanURL] = useState('')
  const [description, setDescription] = useState('')
  const [enabled, setEnabled] = useState(true)
  const [visibility, setVisibility] = useState('public')
  const [openMode, setOpenMode] = useState('new_tab')
  const [linkedContainer, setLinkedContainer] = useState('')
  const [containerQuery, setContainerQuery] = useState('')
  const [dockerList, setDockerList] = useState<DockerContainersInfo | null>(null)
  const [dockerLoading, setDockerLoading] = useState(false)
  const [dockerReadError, setDockerReadError] = useState('')
  const [iconChoice, setIconChoice] = useState<IconChoice>('none')
  const [iconValue, setIconValue] = useState('')
  const [pendingFile, setPendingFile] = useState<File | undefined>()
  const [preview, setPreview] = useState<string | undefined>()
  const [error, setError] = useState('')
  const fileInput = useRef<HTMLInputElement>(null)
  const firstField = useRef<HTMLInputElement>(null)

  useEffect(() => {
    if (!open) return
    const source = initial ?? emptyDraft()
    setName(source.name)
    setLanURL(source.lan_url)
    setWanURL(source.wan_url)
    setDescription(source.description)
    setEnabled(source.enabled)
    setVisibility(source.visibility || 'public')
    setOpenMode(source.open_mode || 'new_tab')
    setLinkedContainer(source.docker_container || '')
    setContainerQuery('')
    setDockerReadError('')
    setIconChoice(choiceFor(source))
    setIconValue(source.icon_value)
    setPendingFile(source.iconBlob)
    setPreview(source.iconPreview || source.icon_url)
    setError('')
    const timer = window.setTimeout(() => firstField.current?.focus(), 60)
    return () => window.clearTimeout(timer)
  }, [open, initial])

  useEffect(() => {
    if (!open) return
    const controller = new AbortController()
    setDockerLoading(true)
    setDockerList(null)
    loadDockerContainers(controller.signal)
      .then((data) => { if (!controller.signal.aborted) setDockerList(data) })
      .catch(() => { if (!controller.signal.aborted) setDockerReadError('无法读取容器列表，请稍后重试') })
      .finally(() => { if (!controller.signal.aborted) setDockerLoading(false) })
    return () => controller.abort()
  }, [open])

  useEffect(() => {
    if (!open) return
    const onKey = (event: KeyboardEvent) => {
      if (event.key === 'Escape') onClose()
    }
    document.addEventListener('keydown', onKey)
    const oldOverflow = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    return () => {
      document.removeEventListener('keydown', onKey)
      document.body.style.overflow = oldOverflow
    }
  }, [open, onClose])

  if (!open) return null

  function pickFile(file: File | undefined) {
    setError('')
    if (!file) {
      setPendingFile(undefined)
      setPreview(initial?.icon_url)
      return
    }
    if (!file.type.startsWith('image/')) {
      setError('图标必须是图片文件')
      return
    }
    if (file.size > 256 * 1024) {
      setError('图标不能超过 256 KiB')
      return
    }
    const reader = new FileReader()
    reader.onload = () => {
      setPendingFile(file)
      setPreview(typeof reader.result === 'string' ? reader.result : undefined)
    }
    reader.onerror = () => setError('读取图标文件失败')
    reader.readAsDataURL(file)
  }

  function submit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault()
    setError('')
    if (!name.trim()) {
      setError('请填写名称')
      return
    }
    if (name.trim().length > 64) {
      setError('名称最多 64 个字符')
      return
    }
    const lan = lanURL.trim()
    const wan = wanURL.trim()
    if (!lan && !wan) {
      setError('LAN URL 和 WAN URL 至少填写一个')
      return
    }
    if (lan && !isHTTPURL(lan)) {
      setError('LAN URL 必须是 http 或 https 地址')
      return
    }
    if (wan && !isHTTPURL(wan)) {
      setError('WAN URL 必须是 http 或 https 地址')
      return
    }
    if (description.length > 200) {
      setError('描述最多 200 个字符')
      return
    }
    if (iconChoice === 'text' && !iconValue.trim()) {
      setError('文字图标不能为空')
      return
    }
    if (iconChoice === 'url' && !isHTTPURL(iconValue.trim())) {
      setError('图标链接必须是 http 或 https 地址')
      return
    }
    if (iconChoice === 'file' && !pendingFile && !(initial?.icon_type === 'file' && initial.has_icon)) {
      setError('请选择一张图标图片')
      return
    }

    const source = initial ?? emptyDraft()
    const iconType =
      iconChoice === 'file' ? 'file' : iconChoice === 'text' ? (source.icon_type === 'emoji' ? 'emoji' : 'text') : iconChoice
    const iconVal = iconChoice === 'text' ? iconValue.trim() : iconChoice === 'url' ? iconValue.trim() : ''
    const next: DraftBookmark = {
      ...source,
      name: name.trim(),
      lan_url: lan,
      wan_url: wan,
      description: description.trim(),
      enabled,
      visibility,
      open_mode: openMode,
      docker_container: linkedContainer,
      icon_type: iconType,
      icon_value: iconVal,
      has_icon: iconChoice === 'file' ? Boolean(pendingFile || source.has_icon) : false,
      icon_url: iconChoice === 'file' ? source.icon_url : undefined,
      iconBlob: iconChoice === 'file' ? pendingFile : undefined,
      iconPreview: iconChoice === 'file' ? preview : undefined,
      isNew: source.isNew,
    }
    onSubmit(next)
  }

  return (
    <div className="drawer-layer" role="presentation" onMouseDown={(event) => event.target === event.currentTarget && onClose()}>
      <aside className="bookmark-drawer" role="dialog" aria-modal="true" aria-labelledby="drawer-title">
        <header className="drawer-head">
          <div>
            <p className="eyebrow">BOOKMARK EDITOR</p>
            <h2 id="drawer-title">{initial ? '编辑书签' : '新增书签'}</h2>
          </div>
          <button type="button" className="sheet-close" onClick={onClose} aria-label="关闭编辑抽屉">
            <CloseIcon size={18} />
          </button>
        </header>

        <form className="drawer-form" onSubmit={submit}>
          <label>
            <span>名称 <span className="required-mark">*</span></span>
            <input ref={firstField} value={name} maxLength={64} onChange={(event) => setName(event.target.value)} placeholder="例如：NAS 管理台" />
          </label>

          <div className="drawer-row">
            <label>
              LAN URL
              <input value={lanURL} onChange={(event) => setLanURL(event.target.value)} placeholder="http://192.168.1.10:5000" spellCheck={false} />
            </label>
            <label>
              WAN URL
              <input value={wanURL} onChange={(event) => setWanURL(event.target.value)} placeholder="https://nas.example.com" spellCheck={false} />
            </label>
          </div>
          <p className="field-hint">至少填写一个；优先使用 WAN，无法访问时回退 LAN。仅允许 http/https。</p>

          <label>
            描述
            <textarea value={description} maxLength={200} rows={3} onChange={(event) => setDescription(event.target.value)} placeholder="可选，最多 200 字" />
          </label>

          <fieldset className="docker-link-fieldset">
            <legend>Docker 容器关联</legend>
            <div className="docker-link-current" aria-live="polite">
              {dockerLoading ? (
                <span>正在读取容器列表…</span>
              ) : dockerReadError ? (
                <span className="docker-link-note">{dockerReadError}；仅保存本批前请确认关联。</span>
              ) : !linkedContainer ? (
                <span>未关联</span>
              ) : dockerList?.status !== 'connected' ? (
                <span className="docker-link-note">Docker 暂不可用，无法确认容器是否仍然存在；当前关联不会被自动移除。</span>
              ) : (() => {
                const c = dockerList.containers.find((item) => item.id === linkedContainer)
                return c ? (
                  <span className="docker-link-current-row">
                    <span className="docker-container-dot" data-tone={containerStatusTone(c.status)} aria-hidden="true" />
                    <strong>{c.name}</strong>
                    <span>{c.image}</span>
                    <span>{c.id.slice(0, 12)}</span>
                    <span>{containerStatusLabel(c.status)}</span>
                  </span>
                ) : (
                  <span className="docker-link-missing">容器已不存在（关联 ID {linkedContainer.slice(0, 12)}）</span>
                )
              })()}
            </div>
            <div className="docker-link-controls">
              <input
                value={containerQuery}
                onChange={(event) => setContainerQuery(event.target.value)}
                placeholder="搜索容器名称、镜像或 ID"
                spellCheck={false}
                disabled={dockerLoading || dockerList?.status !== 'connected'}
              />
              {linkedContainer && (
                <button type="button" className="ghost-button" onClick={() => setLinkedContainer('')}>
                  解除关联
                </button>
              )}
            </div>
            {dockerList?.status === 'connected' ? (
              <div className="docker-link-list" role="listbox" aria-label="可关联 Docker 容器">
                {dockerList.containers
                  .filter((c) => {
                    const q = containerQuery.trim().toLowerCase()
                    return !q || c.name.toLowerCase().includes(q) || c.image.toLowerCase().includes(q) || c.id.toLowerCase().includes(q)
                  })
                  .slice(0, 50)
                  .map((c) => (
                    <button
                      type="button"
                      key={c.id}
                      className={`docker-link-option${linkedContainer === c.id ? ' is-active' : ''}`}
                      aria-pressed={linkedContainer === c.id}
                      onClick={() => setLinkedContainer(c.id)}
                    >
                      <span className="docker-container-dot" data-tone={containerStatusTone(c.status)} aria-hidden="true" />
                      <span className="docker-link-option-main">
                        <strong>{c.name}</strong>
                        <span>{c.image} · {c.id.slice(0, 12)}</span>
                      </span>
                      <span className={`docker-link-state docker-link-state--${containerStatusTone(c.status)}`}>{containerStatusLabel(c.status)}</span>
                    </button>
                  ))}
                {dockerList.containers.length === 0 && <span className="docker-link-note">当前 Docker 环境没有可关联的容器。</span>}
              </div>
            ) : dockerList ? (
              <p className="docker-link-note">{dockerList.message || 'Docker 不可用，暂不能选择容器'}</p>
            ) : null}
            <p className="field-hint">关联仅保存容器 ID，不会自动迁移到同名新容器。本功能只读，不提供启停、删除或日志操作。</p>
          </fieldset>

          <fieldset className="icon-fieldset">
            <legend>图标</legend>
            <div className="icon-choice-row" role="group" aria-label="图标类型">
              {iconChoices.map((item) => (
                <button
                  key={item.value}
                  type="button"
                  className={`icon-choice${iconChoice === item.value ? ' is-active' : ''}`}
                  aria-pressed={iconChoice === item.value}
                  onClick={() => {
                    setIconChoice(item.value)
                    setError('')
                  }}
                >
                  {item.label}
                </button>
              ))}
            </div>
            <div className="icon-editor">
              <span className="icon-preview" aria-hidden="true">
                {preview && (iconChoice === 'file' || iconChoice === 'url' || iconChoice === 'favicon') ? (
                  <img src={preview} alt="" onError={() => setPreview(undefined)} />
                ) : iconChoice === 'text' && iconValue.trim() ? (
                  <span>{iconValue.trim()}</span>
                ) : (
                  <span>{initialLetter(name || '·')}</span>
                )}
              </span>
              <div className="icon-editor-fields">
                {iconChoice === 'text' && (
                  <input value={iconValue} maxLength={8} onChange={(event) => setIconValue(event.target.value)} placeholder="文字或 Emoji" />
                )}
                {iconChoice === 'url' && (
                  <input value={iconValue} onChange={(event) => setIconValue(event.target.value)} placeholder="https://example.com/icon.png" spellCheck={false} />
                )}
                {iconChoice === 'favicon' && <p className="field-hint">保存后会从书签 URL 自动抓取网站图标。</p>}
                {iconChoice === 'file' && (
                  <>
                    <input
                      ref={fileInput}
                      className="visually-hidden"
                      type="file"
                      accept="image/*"
                      onChange={(event) => pickFile(event.target.files?.[0])}
                    />
                    <button type="button" className="quiet-button file-button" onClick={() => fileInput.current?.click()}>
                      选择图片
                    </button>
                    <p className="field-hint">PNG/JPEG/WebP，最大 256 KiB。保存前仅在本地预览。</p>
                  </>
                )}
                {iconChoice === 'none' && <p className="field-hint">使用名称首字母作为占位图标。</p>}
              </div>
            </div>
          </fieldset>

          <div className="drawer-row drawer-row--three">
            <label>
              权限
              <select value={visibility} onChange={(event) => setVisibility(event.target.value)}>
                <option value="public">公开</option>
                <option value="authenticated">登录可见</option>
                <option value="private">私有</option>
              </select>
            </label>
            <label>
              打开方式
              <select value={openMode} onChange={(event) => setOpenMode(event.target.value)}>
                <option value="new_tab">新标签页</option>
                <option value="self">当前标签页</option>
              </select>
            </label>
            <label className="switch-field">
              启用
              <span className="switch-line">
                <input type="checkbox" checked={enabled} onChange={(event) => setEnabled(event.target.checked)} />
                <span>{enabled ? '显示' : '隐藏'}</span>
              </span>
            </label>
          </div>

          {currentSize && <p className="field-hint">当前设备卡片尺寸：{currentSize === 'large' ? '大' : currentSize === 'medium' ? '中' : '小'}，可在卡片上调整。</p>}
          {error && <p className="form-message form-message--error" role="alert">{error}</p>}

          <footer className="drawer-actions">
            {initial && onRequestDelete && (
              <button type="button" className="quiet-button danger-text drawer-delete" onClick={() => onRequestDelete(initial)}>
                删除书签
              </button>
            )}
            <span className="drawer-actions-spacer" />
            <button type="button" className="quiet-button" onClick={onClose}>取消</button>
            <button type="submit" className="primary-button">{initial ? '应用修改' : '加入预览'}</button>
          </footer>
        </form>
      </aside>
    </div>
  )
}

export type { BookmarkInput }
