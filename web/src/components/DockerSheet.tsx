import { useCallback, useEffect, useRef, useState } from 'react'
import { APIError, loadDockerStatus } from '../api'
import { dockerStatusDescription, dockerStatusLabel, dockerStatusTone, formatDockerCheckedAt } from '../docker'
import type { DockerStatusInfo } from '../types'
import { CloseIcon, RefreshIcon, ServerIcon, ShieldIcon } from './Icons'

type Props = {
  isAdmin: boolean
  onClose: () => void
}

function messageFor(error: unknown): string {
  if (error instanceof APIError) {
    if (error.status === 401 || error.status === 403) return '管理会话已失效，请重新登录后再查看'
    if (error.status === 503) return '服务暂时不可用，请稍后重试'
    return error.message || '请求失败'
  }
  return '网络异常，请稍后重试'
}

export function DockerSheet({ isAdmin, onClose }: Props) {
  const [status, setStatus] = useState<DockerStatusInfo | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const firstButton = useRef<HTMLButtonElement>(null)

  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      if (event.key === 'Escape') onClose()
    }
    document.addEventListener('keydown', onKey)
    const previousOverflow = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    const timer = window.setTimeout(() => firstButton.current?.focus(), 80)
    return () => {
      document.removeEventListener('keydown', onKey)
      document.body.style.overflow = previousOverflow
      window.clearTimeout(timer)
    }
  }, [onClose])

  const refresh = useCallback(
    async (signal?: AbortSignal, showLoading = true) => {
      if (!isAdmin) return
      if (showLoading) setLoading(true)
      try {
        const value = await loadDockerStatus(signal)
        if (signal?.aborted) return
        setStatus(value)
        setError('')
      } catch (err: unknown) {
        if (signal?.aborted) return
        setError(messageFor(err))
      } finally {
        if (!signal?.aborted) setLoading(false)
      }
    },
    [isAdmin],
  )

  useEffect(() => {
    if (!isAdmin) return
    const controller = new AbortController()
    void refresh(controller.signal)
    return () => controller.abort()
  }, [isAdmin, refresh])

  // 面板打开期间每 20 秒刷新一次；关闭即停止，不增加后台负担。
  useEffect(() => {
    if (!isAdmin) return
    const timer = window.setInterval(() => void refresh(undefined, false), 20000)
    return () => window.clearInterval(timer)
  }, [isAdmin, refresh])

  const tone = status ? dockerStatusTone(status.status) : 'unknown'

  return (
    <div className="sheet-layer" role="presentation" onMouseDown={(event) => event.target === event.currentTarget && onClose()}>
      <div className="account-sheet docker-sheet" role="dialog" aria-modal="true" aria-labelledby="docker-title">
        <div className="sheet-handle" aria-hidden="true" />
        <header className="sheet-head">
          <div>
            <p className="eyebrow">LOCALHUB DOCKER</p>
            <h2 id="docker-title">Docker 连接</h2>
          </div>
          <button type="button" className="sheet-close" onClick={onClose} aria-label="关闭 Docker 连接面板">
            <CloseIcon size={18} />
          </button>
        </header>

        {!isAdmin ? (
          <section className="docker-empty" aria-label="Docker 状态需要管理员权限">
            <span className="docker-empty-icon" aria-hidden="true">
              <ShieldIcon size={22} />
            </span>
            <strong>仅管理员可查看</strong>
            <p>Docker 连接状态属于基础设施信息，请使用管理员账户登录后查看。</p>
          </section>
        ) : (
          <>
            {loading && !status && (
              <div className="docker-skeleton" aria-busy="true" aria-label="正在检测 Docker 连接">
                <span />
                <span />
              </div>
            )}

            {error && !status && (
              <div className="network-targets-error" role="alert">
                <strong>无法读取 Docker 状态</strong>
                <p>{error}</p>
                <button type="button" className="ghost-button" onClick={() => void refresh()}>
                  重试
                </button>
              </div>
            )}

            {status && (
              <section className="docker-state-card" data-tone={tone} aria-live="polite">
                <span className="docker-state-icon" aria-hidden="true">
                  <ServerIcon size={21} />
                </span>
                <div className="docker-state-copy">
                  <span className="docker-state-kicker">
                    <span className="docker-state-dot" data-tone={tone} aria-hidden="true" />
                    {dockerStatusLabel(status.status)}
                  </span>
                  <strong>{dockerStatusDescription(status.status)}</strong>
                </div>
                <span className="docker-readonly-chip">只读</span>
              </section>
            )}

            {status && (
              <dl className="docker-meta">
                <div>
                  <dt>API 版本</dt>
                  <dd>{status.status === 'connected' && status.api_version ? status.api_version : '—'}</dd>
                </div>
                <div>
                  <dt>最近检测</dt>
                  <dd>{formatDockerCheckedAt(status.checked_at)}</dd>
                </div>
              </dl>
            )}

            {error && status && (
              <p className="form-message form-message--error" role="alert">{error}</p>
            )}

            <div className="docker-actions">
              <button ref={firstButton} type="button" className="primary-button" onClick={() => void refresh()} disabled={loading}>
                <RefreshIcon size={16} />
                {loading ? '正在检测…' : '重新检测'}
              </button>
            </div>

            <p className="network-footnote network-footnote--muted">
              LocalHub 仅通过 Docker Socket Proxy 访问只读 API；不提供容器启停、删除、创建、命令执行，也不提供镜像、网络或卷管理入口。
            </p>
          </>
        )}
      </div>
    </div>
  )
}
