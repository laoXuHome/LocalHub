import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  AUTO_CACHE_TTL_MS,
  browserProbeTarget,
  buildProbeURLs,
  createAutoDetector,
  detectNetwork,
  isPrivateHost,
} from './autodetect'
import type { NetworkProbeTarget } from './types'

const router: NetworkProbeTarget = { key: 'router', address: 'router.local' }
const bypass: NetworkProbeTarget = { key: 'bypass_router', address: '192.168.2.3' }

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('P4.3 probe URLs and target classification', () => {
  it('orders HTTP first on an HTTP page and HTTPS first on an HTTPS page', () => {
    expect(buildProbeURLs('192.168.1.1', 'http:').map((item) => item.scheme)).toEqual(['http', 'https'])
    expect(buildProbeURLs('router.local', 'https:').map((item) => item.scheme)).toEqual(['https', 'http'])
  })

  it('wraps IPv6 targets and avoids double brackets', () => {
    expect(buildProbeURLs('fd00::1', 'http:')[0].url).toBe('http://[fd00::1]/')
    expect(buildProbeURLs('[fd00::1]', 'http:')[0].url).toBe('http://[fd00::1]/')
  })

  it('recognizes private and public hosts', () => {
    expect(isPrivateHost('router.local')).toBe(true)
    expect(isPrivateHost('nas')).toBe(true)
    expect(isPrivateHost('192.168.1.1')).toBe(true)
    expect(isPrivateHost('10.0.0.5')).toBe(true)
    expect(isPrivateHost('172.20.1.1')).toBe(true)
    expect(isPrivateHost('fd00::1')).toBe(true)
    expect(isPrivateHost('example.com')).toBe(false)
    expect(isPrivateHost('router.example.com')).toBe(false)
  })
})

describe('P4.3 browser HTTP probe', () => {
  it('treats any reachable no-cors HTTP response as success', async () => {
    const fetchMock = vi.fn().mockResolvedValue({})
    vi.stubGlobal('fetch', fetchMock)
    await expect(browserProbeTarget(router, 40)).resolves.toBe('success')
    expect(fetchMock).toHaveBeenCalledTimes(1)
    expect(fetchMock.mock.calls[0][1].redirect).toBeUndefined()
  })

  it('treats a plain network error on both schemes as an explicit failure', async () => {
    const fetchMock = vi.fn().mockRejectedValue(new TypeError('Failed to fetch'))
    vi.stubGlobal('fetch', fetchMock)
    await expect(browserProbeTarget(router, 40)).resolves.toBe('failure')
    expect(fetchMock).toHaveBeenCalledTimes(2)
  })

  it('keeps a browser SecurityError as UNKNOWN instead of WAN', async () => {
    const fetchMock = vi.fn().mockRejectedValue(new DOMException('blocked', 'SecurityError'))
    vi.stubGlobal('fetch', fetchMock)
    await expect(browserProbeTarget(router, 40)).resolves.toBe('unknown')
    expect(fetchMock).toHaveBeenCalledTimes(2)
  })

  it('marks mixed content / PNA on an HTTPS public page as UNKNOWN', async () => {
    vi.stubGlobal('window', {
      location: { protocol: 'https:', hostname: 'localhub.example.com' },
    })
    const fetchMock = vi.fn().mockRejectedValue(new TypeError('blocked'))
    vi.stubGlobal('fetch', fetchMock)
    await expect(browserProbeTarget(bypass, 40)).resolves.toBe('unknown')
    expect(fetchMock).toHaveBeenCalledTimes(1)
    expect(fetchMock.mock.calls[0][0]).toBe('https://192.168.2.3/')
  })

  it('keeps an HTTPS-page public target failure as UNKNOWN because HTTP fallback is mixed content', async () => {
    vi.stubGlobal('window', {
      location: { protocol: 'https:', hostname: 'localhub.example.com' },
    })
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new TypeError('offline')))
    await expect(browserProbeTarget({ key: 'router', address: 'router.example.com' }, 40)).resolves.toBe(
      'unknown',
    )
  })
})

describe('P4.3 two-round AUTO detection', () => {
  it('returns UNKNOWN without requesting when no target is enabled', async () => {
    const probe = vi.fn()
    const result = await detectNetwork([], { probe })
    expect(result.status).toBe('unknown')
    expect(result.reason).toBe('no_targets')
    expect(result.rounds).toBe(0)
    expect(probe).not.toHaveBeenCalled()
  })

  it('returns LAN immediately when any enabled target succeeds', async () => {
    const result = await detectNetwork([router, bypass], {
      probe: async (target) => (target.key === 'bypass_router' ? 'success' : 'failure'),
    })
    expect(result.status).toBe('lan')
    expect(result.rounds).toBe(1)
  })

  it('lets a failing target exception coexist with another target success', async () => {
    const result = await detectNetwork([router, bypass], {
      probe: async (target) => {
        if (target.key === 'router') throw new Error('boom')
        await new Promise((resolve) => setTimeout(resolve, 15))
        return 'success'
      },
    })
    expect(result.status).toBe('lan')
    expect(result.rounds).toBe(1)
  })

  it('aborts the remaining targets once a target succeeds', async () => {
    let pendingSignal: AbortSignal | undefined
    const result = await detectNetwork([router, bypass], {
      probe: async (target, _timeout, signal) => {
        if (target.key === 'router') return 'success'
        pendingSignal = signal
        return new Promise((resolve) => {
          signal?.addEventListener('abort', () => resolve('failure'), { once: true })
        })
      },
    })
    expect(result.status).toBe('lan')
    expect(pendingSignal?.aborted).toBe(true)
  })

  it('runs a second round after round one all fails and still finds LAN', async () => {
    const timeouts: number[] = []
    const result = await detectNetwork([router, bypass], {
      probe: async (target, timeout) => {
        timeouts.push(timeout)
        if (timeout === 800) return 'failure'
        return target.key === 'bypass_router' ? 'success' : 'failure'
      },
    })
    expect(result.status).toBe('lan')
    expect(result.rounds).toBe(2)
    expect(timeouts).toContain(800)
    expect(timeouts).toContain(500)
  })

  it('returns WAN only after both rounds explicitly fail', async () => {
    const timeouts: number[] = []
    const result = await detectNetwork([router, bypass], {
      probe: async (_target, timeout) => {
        timeouts.push(timeout)
        return 'failure'
      },
    })
    expect(result.status).toBe('wan')
    expect(result.reason).toBe('all_failed')
    expect(result.rounds).toBe(2)
    expect(timeouts.filter((value) => value === 800)).toHaveLength(2)
    expect(timeouts.filter((value) => value === 500)).toHaveLength(2)
  })

  it('keeps a first-round UNKNOWN and does not run round two', async () => {
    const timeouts: number[] = []
    const result = await detectNetwork([router], {
      probe: async (_target, timeout) => {
        timeouts.push(timeout)
        return 'unknown'
      },
    })
    expect(result.status).toBe('unknown')
    expect(result.reason).toBe('browser_restricted')
    expect(result.rounds).toBe(1)
    expect(timeouts).toEqual([800])
  })

  it('never converts a second-round UNKNOWN into WAN', async () => {
    const result = await detectNetwork([router, bypass], {
      probe: async (target, timeout) => {
        if (timeout === 800) return 'failure'
        return target.key === 'router' ? 'unknown' : 'failure'
      },
    })
    expect(result.status).toBe('unknown')
    expect(result.reason).toBe('browser_restricted')
    expect(result.rounds).toBe(2)
  })
})

describe('P4.3 auto coordinator', () => {
  it('limits duplicate automatic runs and allows a forced recheck', async () => {
    let clock = 1_000
    let probes = 0
    const detector = createAutoDetector({
      loadTargets: async () => ({ targets: [router] }),
      probe: async () => {
        probes += 1
        return 'success'
      },
      now: () => clock,
      minIntervalMs: 3_000,
    })
    await detector.run(false)
    expect(detector.getSnapshot().result?.status).toBe('lan')
    clock += 1_000
    await detector.run(false)
    expect(probes).toBe(1)
    await detector.run(true)
    expect(probes).toBe(2)
  })

  it('caches a result and rechecks after the TTL', async () => {
    let clock = 1_000
    let probes = 0
    const detector = createAutoDetector({
      loadTargets: async () => ({ targets: [router] }),
      probe: async () => {
        probes += 1
        return 'failure'
      },
      now: () => clock,
      minIntervalMs: 3_000,
    })
    await detector.run(false)
    clock += AUTO_CACHE_TTL_MS
    await detector.run(false)
    expect(probes).toBe(4)
    expect(detector.getSnapshot().phase).toBe('checked')
  })

  it('shares one in-flight run', async () => {
    let release: (value: { targets: NetworkProbeTarget[] }) => void = () => {}
    let loads = 0
    let probes = 0
    const detector = createAutoDetector({
      loadTargets: () =>
        new Promise((resolve) => {
          loads += 1
          release = resolve
        }),
      probe: async () => {
        probes += 1
        return 'success'
      },
    })
    const first = detector.run(true)
    const second = detector.run(true)
    expect(loads).toBe(1)
    release({ targets: [router] })
    await Promise.all([first, second])
    expect(probes).toBe(1)
  })

  it('returns UNKNOWN while the browser is offline without probing', async () => {
    let probes = 0
    const detector = createAutoDetector({
      loadTargets: async () => ({ targets: [router] }),
      probe: async () => {
        probes += 1
        return 'success'
      },
      isOnline: () => false,
    })
    await detector.run(false)
    expect(detector.getSnapshot().result?.status).toBe('unknown')
    expect(detector.getSnapshot().result?.reason).toBe('offline')
    expect(probes).toBe(0)
  })

  it('returns UNKNOWN when target loading fails', async () => {
    const detector = createAutoDetector({
      loadTargets: async () => {
        throw new Error('network down')
      },
      probe: async () => 'success',
    })
    await detector.run(false)
    expect(detector.getSnapshot().result?.status).toBe('unknown')
    expect(detector.getSnapshot().result?.reason).toBe('targets_unavailable')
  })

  it('invalidates to UNKNOWN and can run again after an offline transition', async () => {
    let online = true
    let probes = 0
    const detector = createAutoDetector({
      loadTargets: async () => ({ targets: [router] }),
      probe: async () => {
        probes += 1
        return online ? 'success' : 'failure'
      },
      isOnline: () => online,
    })
    await detector.run(false)
    detector.invalidate('offline')
    expect(detector.getSnapshot().result?.status).toBe('unknown')
    expect(detector.getSnapshot().result?.reason).toBe('offline')
    online = true
    await detector.run(false)
    expect(probes).toBe(2)
    expect(detector.getSnapshot().result?.status).toBe('lan')
  })
})
