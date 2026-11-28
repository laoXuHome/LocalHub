import { describe, expect, it } from 'vitest'
import {
  containerStatusLabel,
  containerStatusTone,
  dockerStatusDescription,
  dockerStatusLabel,
  dockerStatusTone,
  formatDockerCheckedAt,
  formatDockerCreatedAt,
  formatDockerStartedAt,
} from './docker'

describe('P5.1 Docker 状态映射', () => {
  it('五种业务状态和未知状态映射到固定色调', () => {
    expect(dockerStatusTone('connected')).toBe('connected')
    expect(dockerStatusTone('not_connected')).toBe('not_connected')
    expect(dockerStatusTone('connection_failed')).toBe('fault')
    expect(dockerStatusTone('permission_denied')).toBe('fault')
    expect(dockerStatusTone('service_unavailable')).toBe('fault')
    expect(dockerStatusTone('unknown')).toBe('unknown')
  })

  it('状态文案可区分连接失败与权限不足', () => {
    expect(dockerStatusLabel('connected')).toBe('已连接')
    expect(dockerStatusLabel('not_connected')).toBe('未连接')
    expect(dockerStatusLabel('connection_failed')).toBe('连接失败')
    expect(dockerStatusLabel('permission_denied')).toBe('权限不足')
    expect(dockerStatusLabel('service_unavailable')).toBe('服务不可用')
    expect(dockerStatusLabel('unknown')).toBe('未知')
    expect(dockerStatusDescription('permission_denied')).toContain('拒绝')
  })

  it('检测时间非法时显示占位符', () => {
    expect(formatDockerCheckedAt('')).toBe('—')
    expect(formatDockerCheckedAt('not-a-date')).toBe('—')
  })
})

describe('P5.2 容器状态映射', () => {
  it('四种容器状态映射到固定色调', () => {
    expect(containerStatusTone('running')).toBe('connected')
    expect(containerStatusTone('stopped')).toBe('not_connected')
    expect(containerStatusTone('error')).toBe('fault')
    expect(containerStatusTone('unknown')).toBe('unknown')
  })

  it('容器状态文案与时间占位符', () => {
    expect(containerStatusLabel('running')).toBe('运行中')
    expect(containerStatusLabel('stopped')).toBe('已停止')
    expect(containerStatusLabel('error')).toBe('异常')
    expect(containerStatusLabel('unknown')).toBe('未知')
    expect(formatDockerCreatedAt('')).toBe('创建时间未知')
    expect(formatDockerCreatedAt('bad')).toBe('创建时间未知')
    expect(formatDockerStartedAt('bad')).toBe('—')
  })
})
