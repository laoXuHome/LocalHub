import { useCallback, useEffect, useRef, useState } from 'react'
import {
  APIError,
  loadNetworkProbeRun,
  loadNetworkResults,
  loadNetworkStatus,
  loadNetworkTargets,
  saveNetworkTargets,
  triggerNetworkProbe,
} from '../api'
import { formatProbeCheckedAt, formatProbeLatency, probeHTTPLabel, probeStateLabel, probeStateTone } from '../probe'
import { autoReasonText, autoStatusText, bookmarkAddressHint, bookmarkAddressMode, bookmarkAddressShortLabel, networkEntryState, networkModeDescription, networkModeLabel, networkModes } from '../network'
import type {
  NetworkConfig,
  NetworkMode,
  NetworkProbeResult,
  NetworkProbeRun,
  NetworkProbeStatus,
  NetworkTarget,
  NetworkTargetKey,
} from '../types'
import type { AutoDetectionResult } from '../autodetect'
import { CloseIcon, NetworkIcon, RefreshIcon, ShieldIcon } from './Icons'

type Props = {
  mode: NetworkMode
  isAdmin: boolean
  autoPhase: 'idle' | 'probing' | 'checked'
  autoResult: AutoDetectionResult | null
  onRecheck: () => Promise<void> | void
  onModeChange: (mode: NetworkMode) => boolean
  onClose: () => void
}

function messageFor(error: unknown): string {
  if (error instanceof APIError) {
    if (error.status === 401 || error.status === 403) return '管理会话已失效，请重新登录后再保存'
    if (error.status === 503) return '服务暂时不可用，请稍后重试'
    if (error.status === 429) return '操作过于频繁，请稍后再试'
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

function ProbeResults({
  isAdmin,
  status,
  loading,
  error,
  run,
  starting,
  notice,
  onProbe,
}: {
  isAdmin: boolean
  status: NetworkProbeStatus | null
  loading: boolean
  error: string
  run: NetworkProbeRun | null
  starting: boolean
  notice: string
  onProbe: () => void
}) {
  const running = starting || run?.status === 'running'
  return (
    <section className="network-section network-probe-section" aria-label="网络探测结果">
      <div className="network-probe-head">
        <div>
          <strong>探测结果</strong>
          <span>最近一次手动探测，不代表当前实时可达。</span>
        </div>
        {isAdmin && (
          <button
            type="button"
            className="primary-button network-probe-button"
            onClick={onProbe}
            disabled={running}
          >
            <RefreshIcon size={16} />
            {running ? '正在探测…' : '手动探测'}
          </button>
        )}
      </div>

      {running && (
        <div className="network-probe-running" role="status" aria-live="polite">
          <span className="network-probe-spinner" aria-hidden="true" />
          正在并行探测三个目标，请稍候…
        </div>
      )}
      {loading && (
        <div className="network-targets-loading" aria-busy="true" aria-label="正在读取探测结果">
          <span />
          <span />
          <span />
        </div>
      )}
      {!loading && error && (
        <div className="network-targets-error" role="alert">
          <strong>无法读取探测结果</strong>
          <p>{error}</p>
        </div>
      )}
      {!loading && !error && status && (
        <div className="network-probe-list">
          {status.targets.map((result) => (
            <ProbeRow result={result} isAdmin={isAdmin} key={result.key} />
          ))}
        </div>
      )}
      {notice && <p className="form-message form-message--ok" role="status">{notice}</p>}
      {!isAdmin && (
        <p className="network-footnote network-footnote--muted">
          访客只能看到公开状态，不显示目标地址或内部错误详情。
        </p>
      )}
    </section>
  )
}

function ProbeRow({ result, isAdmin }: { result: NetworkProbeResult; isAdmin: boolean }) {
  const tone = probeStateTone(result.status)
  const latency = result.status === 'success' || tone === 'failure' ? formatProbeLatency(result.latency_ms) : '—'
  const http = probeHTTPLabel(result.http_status)
  return (
    <article className="network-probe-row" data-state={result.status}>
      <span className="network-status-dot" data-tone={tone} aria-hidden="true" />
      <div className="network-probe-copy">
        <strong>{result.label}</strong>
        <span>
          {probeStateLabel(result.status)}
          {http ? ` · ${http}` : ''}
          {latency !== '—' ? ` · ${latency}` : ''}
        </span>
        {isAdmin && result.address && <span className="network-probe-address">{result.address}</span>}
        {isAdmin && result.failure_reason && tone === 'failure' && (
          <span className="network-probe-reason" title={result.failure_reason}>
            {result.failure_reason}
          </span>
        )}
      </div>
      <time className="network-probe-time" dateTime={result.checked_at || undefined}>
        {formatProbeCheckedAt(result.checked_at)}
      </time>
    </article>
  )
}

export function NetworkSheet({ mode, isAdmin, autoPhase, autoResult, onRecheck, onModeChange, onClose }: Props) {
  const autoView = networkEntryState(mode, autoResult?.status ?? null, autoPhase === 'probing')
  const bookmarkAddress = bookmarkAddressMode(mode, autoResult?.status ?? null, autoPhase === 'probing')
  const [tab, setTab] = useState<'mode' | 'results' | 'targets'>('mode')
  const [config, setConfig] = useState<NetworkConfig | null>(null)
  const [loading, setLoading] = useState(false)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')
  const [status, setStatus] = useState<NetworkProbeStatus | null>(null)
  const [statusLoading, setStatusLoading] = useState(true)
  const [statusError, setStatusError] = useState('')
  const [probeError, setProbeError] = useState('')
  const [run, setRun] = useState<NetworkProbeRun | null>(null)
  const [starting, setStarting] = useState(false)
  const [probeNotice, setProbeNotice] = useState('')
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

  const refreshProbe = useCallback(
    async (signal?: AbortSignal, showLoading = true) => {
      if (showLoading) setStatusLoading(true)
      try {
        const value = isAdmin ? await loadNetworkResults(signal) : await loadNetworkStatus(signal)
        if (signal?.aborted) return
        setStatus(value)
        setStatusError('')
      } catch (err: unknown) {
        if (signal?.aborted) return
        setStatusError(messageFor(err))
      } finally {
        if (!signal?.aborted) setStatusLoading(false)
      }
    },
    [isAdmin],
  )

  useEffect(() => {
    const controller = new AbortController()
    void refreshProbe(controller.signal)
    return () => controller.abort()
  }, [refreshProbe])

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

  // 触发后轮询单次执行记录；完成后刷新最新结果。
  useEffect(() => {
    if (!run || run.status !== 'running') return
    let cancelled = false
    let timer = 0
    const startedAt = Date.now()
    const controller = new AbortController()
    const poll = async () => {
      if (Date.now() - startedAt > 15000) {
        if (!cancelled) {
          setProbeError('探测状态读取超时，请重新打开面板查看结果')
          setStarting(false)
          setRun(null)
        }
        return
      }
      try {
        const next = await loadNetworkProbeRun(run.id, controller.signal)
        if (cancelled) return
        setRun(next)
        if (next.status === 'completed') {
          setStarting(false)
          const successes = next.results.filter((item) => item.status === 'success').length
          setProbeNotice(`探测完成：${successes}/${next.results.length} 个目标成功`)
          void refreshProbe(undefined, false)
          return
        }
        timer = window.setTimeout(poll, 700)
      } catch (err: unknown) {
        if (cancelled) return
        setProbeError(messageFor(err))
        setStarting(false)
        setRun(null)
      }
    }
    timer = window.setTimeout(poll, 300)
    return () => {
      cancelled = true
      controller.abort()
      window.clearTimeout(timer)
    }
  }, [run?.id, run?.status, refreshProbe])

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
      if (mode === 'auto') void onRecheck()
      void refreshProbe(undefined, false)
    } catch (err) {
      setError(messageFor(err))
    } finally {
      setSaving(false)
    }
  }

  async function startProbe() {
    if (starting || run?.status === 'running') return
    setStarting(true)
    setProbeError('')
    setProbeNotice('')
    setTab('results')
    try {
      const started = await triggerNetworkProbe()
      setRun(started)
    } catch (err: unknown) {
      setStarting(false)
      if (err instanceof APIError && err.status === 409) {
        setProbeError('已有探测正在进行，请等待本次完成后再试')
      } else {
        setProbeError(messageFor(err))
      }
    }
  }

  const updated = updatedLabel(config?.updated_at)
  const running = starting || run?.status === 'running'

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

        <div className="network-current" data-tone={mode === 'auto' ? autoView.tone : mode}>
          <span className="network-current-icon" aria-hidden="true">
            <NetworkIcon size={20} />
          </span>
          <div>
            <strong>{networkModeLabel(mode)}</strong>
            <span>
              {mode !== 'auto'
                ? networkModeDescription(mode)
                : autoPhase === 'probing'
                  ? '正在判断当前网络…'
                  : autoResult
                    ? `${autoStatusText(autoResult.status)} · ${autoReasonText(autoResult.reason)}`
                    : '尚未完成自动判断'}
            </span>
          </div>
          <span className={`network-state-dot network-state-dot--${autoView.tone}`} aria-hidden="true" />
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
              aria-selected={tab === 'results'}
              className={tab === 'results' ? 'is-active' : ''}
              onClick={() => setTab('results')}
            >
              探测结果
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
            <div className="network-address-hint" data-mode={bookmarkAddress} role="status">
              <strong>书签打开：{bookmarkAddressShortLabel(bookmarkAddress)}</strong>
              <span>{bookmarkAddressHint(bookmarkAddress)}</span>
            </div>
            <section className="network-auto-card" aria-label="自动网络判断">
              <div className="network-auto-head">
                <div>
                  <strong>自动判断</strong>
                  <span>在当前浏览器执行</span>
                </div>
                <span className={`network-state-dot network-state-dot--${autoView.tone}`} aria-hidden="true" />
              </div>
              <div className="network-auto-result" aria-live="polite">
                <strong className={`network-state-text network-state-text--${autoView.tone}`}>
                  {autoPhase === 'probing' ? '判断中' : autoView.label}
                </strong>
                <span>
                  {mode !== 'auto'
                    ? `当前为手动 ${networkModeLabel(mode)}，自动判断不会覆盖。`
                    : autoPhase === 'probing'
                      ? '首轮 800ms，必要时第二轮 500ms。'
                      : autoResult
                        ? `${autoStatusText(autoResult.status)} · ${autoReasonText(autoResult.reason)}`
                        : '尚未判断。'}
                </span>
              </div>
              {mode === 'auto' && autoResult && (
                <p className="network-footnote network-footnote--muted">
                  最近判断：<time dateTime={autoResult.checkedAt}>{formatProbeCheckedAt(autoResult.checkedAt)}</time>
                  {autoResult.rounds > 0 && ` · ${autoResult.rounds} 轮`}
                </p>
              )}
              {mode === 'auto' && (
                <button
                  type="button"
                  className="quiet-button network-auto-recheck"
                  onClick={() => void onRecheck()}
                  disabled={autoPhase === 'probing'}
                >
                  <RefreshIcon size={14} />
                  {autoPhase === 'probing' ? '判断中…' : '重新判断网络'}
                </button>
              )}
              <p className="network-footnote network-footnote--muted">
                只使用已启用的主路由、旁路由、NAS 做 HTTP/HTTPS 探测；不使用 Ping、不按 IP 子网推断，也不读取 NAS 后端网络状态。
              </p>
            </section>
            <p className="network-footnote">
              手动 LAN / WAN 优先于自动判断。模式仅保存在当前浏览器，不会同步到其他设备；切回 AUTO 后，自动判断才重新接管。
            </p>
            <p className="network-footnote network-footnote--muted">
              P4.2 的管理员手动 HTTP 探测仍保留在「探测结果」中，其语义不变，与本次自动判断相互独立。
            </p>
          </section>
        )}

        {isAdmin && tab === 'results' && (
          <>
            {probeError && <p className="form-message form-message--error" role="alert">{probeError}</p>}
            <ProbeResults
              isAdmin
              status={status}
              loading={statusLoading}
              error={statusError}
              run={run}
              starting={starting}
              notice={probeNotice}
              onProbe={startProbe}
            />
          </>
        )}

        {!isAdmin && tab === 'mode' && (
          <>
            <ProbeResults
              isAdmin={false}
              status={status}
              loading={statusLoading}
              error={statusError}
              run={null}
              starting={false}
              notice=""
              onProbe={() => undefined}
            />
            <div className="guest-note network-guest-note">
              <ShieldIcon size={19} />
              <p>访客可以切换当前浏览器的网络模式，但不能查看探测目标配置，也不能触发探测。</p>
            </div>
          </>
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

        {tab === 'mode' && notice && <p className="form-message form-message--ok">{notice}</p>}
        {tab === 'mode' && error && <p className="form-message form-message--error" role="alert">{error}</p>}
        {running && tab !== 'results' && isAdmin && (
          <p className="network-footnote network-footnote--muted" role="status">探测正在后台执行…</p>
        )}
      </div>
    </div>
  )
}
