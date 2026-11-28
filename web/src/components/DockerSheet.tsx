import { useCallback, useEffect, useRef, useState } from 'react'
import { APIError, loadDockerContainers, loadDockerStatus } from '../api'
import {
  containerStatusLabel,
  containerStatusTone,
  dockerStatusDescription,
  dockerStatusLabel,
  dockerStatusTone,
  formatDockerCheckedAt,
  formatDockerCreatedAt,
  formatDockerStartedAt,
} from '../docker'
import type { DockerContainersInfo, DockerStatusInfo } from '../types'
import { CloseIcon, RefreshIcon, ServerIcon, ShieldIcon } from './Icons'

type Props = {
  isAdmin: boolean
  onClose: () => void
}

type DockerData = {
  status: DockerStatusInfo | null
  list: DockerContainersInfo | null
}

function messageFor(error: unknown): string {
  if (error instanceof APIError) {
    if (error.status === 401 || error.status === 403) return '管理会话已失效，请重新登录后再查看'
    if (error.status === 503) return '服务暂时不可用，请稍后重试'
    return error.message || '请求失败'
  }
  return '网络异常，请稍后重试'
}

function fallbackStatus(list: DockerContainersInfo): DockerStatusInfo {
  return {
    status: list.status,
    checked_at: list.checked_at,
    message: list.message,
    read_only: list.read_only,
  }
}

export function DockerSheet({ isAdmin, onClose }: Props) {
  const [data, setData] = useState<DockerData | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [stale, setStale] = useState(false)
  const firstButton = useRef<HTMLButtonElement>(null)
  const dataRef = useRef<DockerData | null>(null)
  const inFlight = useRef(false)
  const abortRef = useRef<AbortController | null>(null)

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
      abortRef.current?.abort()
    }
  }, [onClose])

  useEffect(() => {
    dataRef.current = data
  }, [data])

  const refresh = useCallback(
    async (showLoading = true) => {
      if (!isAdmin || inFlight.current) return
      inFlight.current = true
      abortRef.current?.abort()
      const controller = new AbortController()
      abortRef.current = controller
      if (showLoading) setLoading(true)
      try {
        const [statusResult, listResult] = await Promise.allSettled([
          loadDockerStatus(controller.signal),
          loadDockerContainers(controller.signal),
        ])
        if (controller.signal.aborted) return

        let nextStatus: DockerStatusInfo | null = null
        let nextList: DockerContainersInfo | null = null
        let failure = ''
        if (statusResult.status === 'fulfilled') nextStatus = statusResult.value
        else failure = messageFor(statusResult.reason)
        if (listResult.status === 'fulfilled') nextList = listResult.value
        else if (!failure) failure = messageFor(listResult.reason)

        const previousList = dataRef.current?.list
        const listConnected = nextList?.status === 'connected'
        const keepPreviousList = Boolean(nextList && !listConnected && previousList?.containers.length)
        if (!failure && nextList && !listConnected) failure = nextList.message || 'Docker 连接异常'
        if (nextList || nextStatus) {
          setData((previous) => ({
            status: nextStatus || (nextList ? fallbackStatus(nextList) : previous?.status || null),
            list: listConnected ? nextList : keepPreviousList ? previous?.list || null : nextList || previous?.list || null,
          }))
        }
        if (failure) {
          // 保留上一次有效容器数据；只标记状态，不把容器改成 stopped。
          setError(failure)
          setStale(Boolean(keepPreviousList || (!nextList && previousList?.containers.length)))
        } else {
          setError('')
          setStale(false)
        }
      } finally {
        inFlight.current = false
        if (!controller.signal.aborted) setLoading(false)
      }
    },
    [isAdmin],
  )

  useEffect(() => {
    if (!isAdmin) return
    void refresh()
  }, [isAdmin, refresh])

  const intervalSeconds = data?.list?.refresh_interval_seconds || 30
  useEffect(() => {
    if (!isAdmin) return
    const seconds = Math.min(300, Math.max(10, intervalSeconds))
    const timer = window.setInterval(() => void refresh(false), seconds * 1000)
    return () => window.clearInterval(timer)
  }, [intervalSeconds, isAdmin, refresh])

  const status = data?.status
  const list = data?.list
  const tone = status ? dockerStatusTone(status.status) : 'unknown'
  const containers = list?.containers || []
  const counts = list?.counts
  const noData = !list && !status

  return (
    <div className="sheet-layer" role="presentation" onMouseDown={(event) => event.target === event.currentTarget && onClose()}>
      <div className="account-sheet docker-sheet" role="dialog" aria-modal="true" aria-labelledby="docker-title">
        <div className="sheet-handle" aria-hidden="true" />
        <header className="sheet-head">
          <div>
            <p className="eyebrow">LOCALHUB DOCKER</p>
            <h2 id="docker-title">Docker 容器</h2>
          </div>
          <button type="button" className="sheet-close" onClick={onClose} aria-label="关闭 Docker 容器面板">
            <CloseIcon size={18} />
          </button>
        </header>

        {!isAdmin ? (
          <section className="docker-empty" aria-label="Docker 数据需要管理员权限">
            <span className="docker-empty-icon" aria-hidden="true">
              <ShieldIcon size={22} />
            </span>
            <strong>仅管理员可查看</strong>
            <p>Docker 容器与连接状态属于基础设施信息，请使用管理员账户登录后查看。</p>
          </section>
        ) : (
          <>
            <section className="docker-state-card" data-tone={tone} aria-live="polite">
              <span className="docker-state-icon" aria-hidden="true">
                <ServerIcon size={21} />
              </span>
              <div className="docker-state-copy">
                <span className="docker-state-kicker">
                  <span className="docker-state-dot" data-tone={tone} aria-hidden="true" />
                  {status ? dockerStatusLabel(status.status) : loading ? '正在检测' : '未知'}
                </span>
                <strong>{status ? dockerStatusDescription(status.status) : '正在读取 Docker 连接状态'}</strong>
              </div>
              <span className="docker-readonly-chip">只读</span>
            </section>

            {stale && (
              <div className="docker-stale" role="alert">
                <strong>当前显示上次有效数据</strong>
                <span>{error || '刷新失败，容器状态可能已变化'}</span>
              </div>
            )}

            <div className="docker-toolbar">
              <div className="docker-summary" aria-label="容器统计">
                <span><b>{counts?.total ?? 0}</b> 个容器</span>
                <span className="docker-summary-running">运行 {counts?.running ?? 0}</span>
                <span className="docker-summary-stopped">停止 {counts?.stopped ?? 0}</span>
                <span className="docker-summary-error">异常 {counts?.error ?? 0}</span>
                {(counts?.unknown ?? 0) > 0 && <span>未知 {counts?.unknown}</span>}
              </div>
              <button
                ref={firstButton}
                type="button"
                className="docker-refresh-button"
                onClick={() => void refresh()}
                disabled={loading}
              >
                <RefreshIcon size={15} />
                {loading ? '刷新中…' : '刷新'}
              </button>
            </div>

            {loading && !list && (
              <div className="docker-skeleton" aria-busy="true" aria-label="正在加载 Docker 容器">
                <span />
                <span />
                <span />
              </div>
            )}

            {!loading && noData && error && (
              <div className="network-targets-error" role="alert">
                <strong>无法读取 Docker 数据</strong>
                <p>{error}</p>
                <button type="button" className="ghost-button" onClick={() => void refresh()}>
                  重试
                </button>
              </div>
            )}

            {list && list.status !== 'connected' && containers.length === 0 && (
              <div className="docker-empty docker-empty--fault">
                <strong>{dockerStatusLabel(list.status)}</strong>
                <p>{list.message || dockerStatusDescription(list.status)}</p>
                {stale && <p>上次有效数据已过期，Docker 恢复后会自动刷新。</p>}
              </div>
            )}

            {list && list.status === 'connected' && containers.length === 0 && (
              <div className="docker-empty">
                <strong>暂无容器</strong>
                <p>当前 Docker 环境没有运行中或已停止的容器。</p>
              </div>
            )}

            {containers.length > 0 && (
              <div className="docker-container-list" aria-label="Docker 容器列表">
                <div className="docker-container-head" aria-hidden="true">
                  <span>容器</span>
                  <span>镜像 / ID</span>
                  <span>端口 / 时间</span>
                </div>
                {containers.map((container) => {
                  const itemTone = containerStatusTone(container.status)
                  return (
                    <article className="docker-container" key={container.id} data-tone={itemTone}>
                      <div className="docker-container-main">
                        <span className="docker-container-dot" data-tone={itemTone} aria-hidden="true" />
                        <div className="docker-container-title">
                          <strong title={container.name}>{container.name}</strong>
                          <span className="docker-container-status">{containerStatusLabel(container.status)}</span>
                        </div>
                      </div>
                      <div className="docker-container-meta">
                        <span className="docker-container-image" title={container.image}>{container.image || '—'}</span>
                        <code title={container.id}>{container.id.slice(0, 12)}</code>
                      </div>
                      <div className="docker-container-extra">
                        <span title={container.ports.join(', ')}>{container.ports.length ? container.ports.join(' · ') : '无端口映射'}</span>
                        <span>{container.status === 'running' && container.started_at ? `运行于 ${formatDockerStartedAt(container.started_at)}` : formatDockerCreatedAt(container.created_at)}</span>
                        {Object.keys(container.labels || {}).length > 0 && <span>{Object.keys(container.labels).length} 个标签</span>}
                      </div>
                    </article>
                  )
                })}
              </div>
            )}

            {error && !stale && !noData && (
              <p className="form-message form-message--error" role="alert">{error}</p>
            )}

            <dl className="docker-meta">
              <div>
                <dt>API 版本</dt>
                <dd>{status?.status === 'connected' && status.api_version ? status.api_version : '—'}</dd>
              </div>
              <div>
                <dt>最近检测</dt>
                <dd>{formatDockerCheckedAt((list || status)?.checked_at)}</dd>
              </div>
              <div>
                <dt>自动刷新</dt>
                <dd>{Math.min(300, Math.max(10, intervalSeconds))} 秒</dd>
              </div>
            </dl>

            {list && list.truncated && (
              <p className="network-footnote network-footnote--muted">容器数量超过上限，列表已截断显示。</p>
            )}
            {list && list.skipped ? (
              <p className="network-footnote network-footnote--muted">已跳过 {list.skipped} 个数据异常的容器，其余列表仍可用。</p>
            ) : null}

            <p className="network-footnote network-footnote--muted">
              LocalHub 仅通过 Docker Socket Proxy 读取容器状态；不提供启停、删除、创建、命令执行、日志、镜像、网络或卷管理入口。标签只作为普通文本数据展示，不执行其中内容。
            </p>
          </>
        )}
      </div>
    </div>
  )
}
