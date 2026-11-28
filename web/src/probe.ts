import type { NetworkProbeState } from './types'

export type ProbeTone = 'success' | 'failure' | 'neutral'

export function probeStateLabel(state: NetworkProbeState): string {
  switch (state) {
    case 'success':
      return '成功'
    case 'http_error':
      return 'HTTP 错误'
    case 'timeout':
      return '超时'
    case 'connection_failed':
      return '连接失败'
    case 'probe_error':
      return '探测异常'
    case 'not_probed':
      return '未探测'
    case 'disabled':
      return '已禁用'
    default:
      return '未知'
  }
}

export function probeStateTone(state: NetworkProbeState): ProbeTone {
  if (state === 'success') return 'success'
  if (state === 'not_probed' || state === 'disabled') return 'neutral'
  return 'failure'
}

export function formatProbeCheckedAt(value?: string): string {
  if (!value) return '—'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return '—'
  return date.toLocaleString('zh-CN', {
    month: 'numeric',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  })
}

export function formatProbeLatency(value: number): string {
  if (!Number.isFinite(value) || value < 0) return '—'
  return `${Math.round(value)} ms`
}

export function probeHTTPLabel(code: number): string {
  return code > 0 ? `HTTP ${code}` : ''
}
