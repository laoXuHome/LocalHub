import { useEffect, useRef, useState } from 'react'
import { APIError, loadNetworkTargets, saveNetworkTargets } from '../api'
import { networkModeDescription, networkModeLabel, networkModes } from '../network'
import type { NetworkConfig, NetworkMode, NetworkTarget, NetworkTargetKey } from '../types'
import { CloseIcon, NetworkIcon, ShieldIcon } from './Icons'

type Props = {
  mode: NetworkMode
  isAdmin: boolean
  onModeChange: (mode: NetworkMode) => boolean
  onClose: () => void
}

function messageFor(error: unknown): string {
  if (error instanceof APIError) {
    if (error.status === 401 || error.status === 403) return '管理会话已失效，请重新登录后再保存'
    if (error.status === 503) return '服务暂时不可用，请稍后重试'
    return error.message || '请求失败'
  }
  return '网络异常，请稍后重试'
}

function updatedLabel(value?: string): string {
  if (!value) return ''
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return ''
  return date.toLocaleString('zh-CN', { month: 'numeric', day: 'numeric', hour: '2-digit', minute: '2-digit' })
}

export function NetworkSheet({ mode, isAdmin, onModeChange, onClose }: Props) {
  const [tab, setTab] = useState<'mode' | 'targets'>('mode')
  const [config, setConfig] = useState<NetworkConfig | null>(null)
  const [loading, setLoading] = useState(false)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')
  const firstMode = useRef<HTMLButtonElement>(null)

  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      if (event.key === 'Escape') onClose()
    }
    document.addEventListener('keydown', onKey)
    const previousOverflow = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    const timer = window.setTimeout(() => firstMode.current?.focus(), 80)
    return () => {
      document.removeEventListener('keydown', onKey)
      document.body.style.overflow = previousOverflow
      window.clearTimeout(timer)
    }
  }, [onClose])

  useEffect(() => {
    if (!isAdmin) return
    const controller = new AbortController()
    setLoading(true)
    setError('')
    loadNetworkTargets(controller.signal)
      .then((value) => setConfig(value))
      .catch((err: unknown) => {
        if (controller.signal.aborted) return
        setError(messageFor(err))
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false)
      })
    return () => controller.abort()
  }, [isAdmin])

  function chooseMode(next: NetworkMode) {
    const stored = onModeChange(next)
    if (!stored) {
      setError('浏览器存储不可用，模式仅在本次会话有效')
      setNotice('')
      return
    }
    setError('')
    setNotice(`已切换为 ${networkModeLabel(next)}，仅保存于当前浏览器`)
  }

  function updateTarget(key: NetworkTargetKey, patch: Partial<NetworkTarget>) {
    setNotice('')
    setConfig((current) => {
      if (!current) return current
      return {
        ...current,
        targets: current.targets.map((target) => (target.key === key ? { ...target, ...patch } : target)),
      }
    })
  }

  async function submitTargets(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (!config) return
    const enabled = config.targets.filter((target) => target.enabled)
    if (enabled.length === 0) {
      setError('至少需要启用一个探测目标')
      return
    }
    const missing = enabled.find((target) => target.address.trim() === '')
    if (missing) {
      setError(`请填写「${missing.label}」地址，或关闭该目标`)
      return
    }

    setSaving(true)
    setError('')
    setNotice('')
    try {
      const saved = await saveNetworkTargets(
        config.targets.map((target) => ({
          key: target.key,
          address: target.address.trim(),
          enabled: target.enabled,
        })),
      )
      setConfig(saved)
      setNotice('探测目标已保存')
    } catch (err) {
      setError(messageFor(err))
    } finally {
      setSaving(false)
    }
  }

  const updated = updatedLabel(config?.updated_at)

  return (
    <div className="sheet-layer" role="presentation" onMouseDown={(event) => event.target === event.currentTarget && onClose()}>
      <div className="account-sheet network-sheet" role="dialog" aria-modal="true" aria-labelledby="network-title">
        <div className="sheet-handle" aria-hidden="true" />
        <header className="sheet-head">
          <div>
            <p className="eyebrow">LOCALHUB NETWORK</p>
            <h2 id="network-title">网络模式</h2>
          </div>
          <button type="button" className="sheet-close" onClick={onClose} aria-label="关闭网络设置">
            <CloseIcon size={18} />
          </button>
        </header>

        <div className="network-current">
          <span className="network-current-icon" aria-hidden="true">
            <NetworkIcon size={20} />
          </span>
          <div>
            <strong>{networkModeLabel(mode)}</strong>
            <span>{networkModeDescription(mode)}</span>
          </div>
          <span className="network-local-chip">本机</span>
        </div>

        {isAdmin && (
          <div className="network-tabs" role="tablist" aria-label="网络设置分类">
            <button
              type="button"
              role="tab"
              aria-selected={tab === 'mode'}
              className={tab === 'mode' ? 'is-active' : ''}
              onClick={() => setTab('mode')}
            >
              模式
            </button>
            <button
              type="button"
              role="tab"
              aria-selected={tab === 'targets'}
              className={tab === 'targets' ? 'is-active' : ''}
              onClick={() => setTab('targets')}
            >
              探测目标
            </button>
          </div>
        )}

        {tab === 'mode' && (
          <section className="network-section" aria-label="网络模式切换">
            <div className="network-mode-options" role="radiogroup" aria-label="选择网络模式">
              {networkModes.map((item) => (
                <button
                  key={item}
                  ref={item === 'auto' ? firstMode : undefined}
                  type="button"
                  role="radio"
                  aria-checked={mode === item}
                  className={`network-mode-option${mode === item ? ' is-selected' : ''}`}
                  onClick={() => chooseMode(item)}
                >
                  <strong>{networkModeLabel(item)}</strong>
                  <span>{networkModeDescription(item)}</span>
                </button>
              ))}
            </div>
            <p className="network-footnote">
              手动 LAN / WAN 优先于自动判断。模式仅保存在当前浏览器，不会同步到其他设备；切回 AUTO 后，后续自动探测才可重新接管。
            </p>
            <p className="network-footnote network-footnote--muted">P4.1 不执行实际探测，AUTO 当前表示等待后续自动判断。</p>
          </section>
        )}

        {tab === 'targets' && isAdmin && (
          <section className="network-section" aria-label="网络探测目标配置">
            {loading && (
              <div className="network-targets-loading" aria-busy="true">
                <span />
                <span />
                <span />
              </div>
            )}
            {!loading && !config && (
              <div className="network-targets-error" role="alert">
                <strong>无法读取探测目标</strong>
                <p>{error || '请稍后重试。'}</p>
              </div>
            )}
            {!loading && config && (
              <form className="network-targets-form" onSubmit={submitTargets}>
                {config.targets.map((target) => (
                  <div className="network-target-row" key={target.key}>
                    <label className="network-target-toggle">
                      <input
                        type="checkbox"
                        checked={target.enabled}
                        onChange={(event) => updateTarget(target.key, { enabled: event.target.checked })}
                        aria-label={`启用${target.label}`}
                      />
                      <span className={`network-switch${target.enabled ? ' is-on' : ''}`} aria-hidden="true" />
                      <strong>{target.label}</strong>
                    </label>
                    <input
                      className="network-address-input"
                      type="text"
                      value={target.address}
                      onChange={(event) => updateTarget(target.key, { address: event.target.value })}
                      placeholder={target.enabled ? 'IP 或主机名' : '未启用'}
                      aria-label={`${target.label}地址`}
                      disabled={saving}
                      autoComplete="off"
                      spellCheck={false}
                    />
                  </div>
                ))}
                <p className="network-footnote">地址支持 IPv4、IPv6 或主机名，不要填写 http://、端口或路径。</p>
                {updated && <p className="network-footnote network-footnote--muted">上次保存：{updated}</p>}
                {error && <p className="form-message form-message--error" role="alert">{error}</p>}
                {notice && <p className="form-message form-message--ok">{notice}</p>}
                <button type="submit" className="primary-button network-save" disabled={saving}>
                  {saving ? '正在保存…' : '保存探测目标'}
                </button>
              </form>
            )}
          </section>
        )}

        {!isAdmin && (
          <div className="guest-note network-guest-note">
            <ShieldIcon size={19} />
            <p>访客可以切换当前浏览器的网络模式，但不能查看或修改探测目标；探测目标配置仅管理员可用。</p>
          </div>
        )}

        {tab === 'mode' && notice && <p className="form-message form-message--ok">{notice}</p>}
        {tab === 'mode' && error && <p className="form-message form-message--error" role="alert">{error}</p>}
      </div>
    </div>
  )
}
