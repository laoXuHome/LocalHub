import type { DockerConnectionStatus } from './types'

export type DockerTone = 'connected' | 'not_connected' | 'fault' | 'unknown'

export function dockerStatusTone(status: DockerConnectionStatus): DockerTone {
  switch (status) {
    case 'connected':
      return 'connected'
    case 'not_connected':
      return 'not_connected'
    case 'connection_failed':
    case 'permission_denied':
    case 'service_unavailable':
      return 'fault'
    default:
      return 'unknown'
  }
}

export function dockerStatusLabel(status: DockerConnectionStatus): string {
  switch (status) {
    case 'connected':
      return '已连接'
    case 'not_connected':
      return '未连接'
    case 'connection_failed':
      return '连接失败'
    case 'permission_denied':
      return '权限不足'
    case 'service_unavailable':
      return '服务不可用'
    default:
      return '未知'
  }
}

export function dockerStatusDescription(status: DockerConnectionStatus): string {
  switch (status) {
    case 'connected':
      return '已通过 Docker Socket Proxy 建立只读连接'
    case 'not_connected':
      return '尚未配置 Docker 连接'
    case 'connection_failed':
      return '无法连接 Docker Socket Proxy，请检查部署配置'
    case 'permission_denied':
      return 'Docker Socket Proxy 拒绝了访问，请检查只读权限配置'
    case 'service_unavailable':
      return 'Docker Socket Proxy 或 Docker 服务暂时不可用'
    default:
      return '暂时无法确定 Docker 连接状态'
  }
}

export function formatDockerCheckedAt(value?: string): string {
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
