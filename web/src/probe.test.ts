import { describe, expect, it } from 'vitest'
import { formatProbeCheckedAt, formatProbeLatency, probeHTTPLabel, probeStateLabel, probeStateTone } from './probe'

describe('P4.2 probe presentation', () => {
  it('labels all states without treating disabled or not_probed as failure', () => {
    expect(probeStateLabel('success')).toBe('成功')
    expect(probeStateLabel('http_error')).toBe('HTTP 错误')
    expect(probeStateLabel('timeout')).toBe('超时')
    expect(probeStateLabel('connection_failed')).toBe('连接失败')
    expect(probeStateLabel('probe_error')).toBe('探测异常')
    expect(probeStateLabel('not_probed')).toBe('未探测')
    expect(probeStateLabel('disabled')).toBe('已禁用')
  })

  it('maps success, failure and neutral tones', () => {
    expect(probeStateTone('success')).toBe('success')
    expect(probeStateTone('timeout')).toBe('failure')
    expect(probeStateTone('http_error')).toBe('failure')
    expect(probeStateTone('not_probed')).toBe('neutral')
    expect(probeStateTone('disabled')).toBe('neutral')
  })

  it('formats checked time and rejects invalid values', () => {
    expect(formatProbeCheckedAt(undefined)).toBe('—')
    expect(formatProbeCheckedAt('not-a-date')).toBe('—')
    expect(formatProbeCheckedAt('2026-11-28T02:30:00Z')).not.toBe('—')
  })

  it('formats latency in milliseconds', () => {
    expect(formatProbeLatency(0)).toBe('0 ms')
    expect(formatProbeLatency(12.4)).toBe('12 ms')
    expect(formatProbeLatency(-1)).toBe('—')
    expect(formatProbeLatency(Number.NaN)).toBe('—')
  })

  it('shows HTTP code only when present', () => {
    expect(probeHTTPLabel(200)).toBe('HTTP 200')
    expect(probeHTTPLabel(500)).toBe('HTTP 500')
    expect(probeHTTPLabel(0)).toBe('')
  })
})
