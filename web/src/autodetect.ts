import type { AutoNetworkStatus, NetworkProbeTarget } from './types'

export const AUTO_ROUND_1_TIMEOUT_MS = 800
export const AUTO_ROUND_2_TIMEOUT_MS = 500
export const AUTO_CACHE_TTL_MS = 60_000
export const AUTO_MIN_INTERVAL_MS = 3_000

type ProbeOutcome = 'success' | 'failure' | 'unknown'

export type AutoDetectionReason =
  | 'matched'
  | 'all_failed'
  | 'browser_restricted'
  | 'no_targets'
  | 'offline'
  | 'targets_unavailable'

export type AutoDetectionResult = {
  status: AutoNetworkStatus
  reason: AutoDetectionReason
  rounds: number
  checkedAt: string
  duration_ms: number
}

export type AutoSnapshot = {
  phase: 'idle' | 'probing' | 'checked'
  result: AutoDetectionResult | null
}

export type BrowserPageContext = {
  protocol: string
  hostname: string
}

type ProbeFn = (target: NetworkProbeTarget, timeoutMs: number, signal?: AbortSignal) => Promise<ProbeOutcome>

type DetectionOptions = {
  probe?: ProbeFn
  signal?: AbortSignal
  timeouts?: [number, number]
  now?: () => Date
}

type RoundOutcome = {
  outcome: ProbeOutcome
}

export type AutoDetectorOptions = {
  loadTargets: (signal?: AbortSignal) => Promise<{ targets: NetworkProbeTarget[] }>
  probe?: ProbeFn
  page?: () => BrowserPageContext
  isOnline?: () => boolean
  timeouts?: [number, number]
  minIntervalMs?: number
  now?: () => number
}

const IPV4 = /^(\d{1,3})\.(\d{1,3})\.(\d{1,3})\.(\d{1,3})$/

function ipv4Parts(host: string): number[] | null {
  const match = IPV4.exec(host)
  if (!match) return null
  const parts = match.slice(1).map(Number)
  return parts.some((part) => part > 255) ? null : parts
}

/** 判断目标是否属于常见内网/回环地址；仅用于识别浏览器安全策略下的“不确定”结果。 */
export function isPrivateHost(host: string): boolean {
  const value = host.trim().toLowerCase().replace(/^\[|\]$/g, '').replace(/\.$/, '')
  if (!value) return true
  if (value === 'localhost') return true
  const v4 = ipv4Parts(value)
  if (v4) {
    const [a, b] = v4
    return (
      a === 127 ||
      a === 10 ||
      (a === 172 && b >= 16 && b <= 31) ||
      (a === 192 && b === 168) ||
      (a === 169 && b === 254) ||
      (a === 100 && b >= 64 && b <= 127) ||
      a === 0
    )
  }
  if (value.includes(':')) {
    return (
      value === '::1' ||
      value.startsWith('fe80:') ||
      value.startsWith('fc') ||
      value.startsWith('fd') ||
      value.startsWith('ff')
    )
  }
  return value.endsWith('.local') || !value.includes('.')
}

function currentPage(): BrowserPageContext {
  if (typeof window === 'undefined') return { protocol: 'http:', hostname: '' }
  return { protocol: window.location.protocol, hostname: window.location.hostname }
}

function formatHost(address: string): string {
  const trimmed = address.trim().replace(/\.$/, '')
  if (trimmed.startsWith('[')) return trimmed
  return trimmed.includes(':') ? `[${trimmed}]` : trimmed
}

/** 目标 URL 只由已校验地址和协议组成，不能携带路径或查询参数。 */
export function buildProbeURLs(address: string, pageProtocol: string): Array<{ scheme: 'http' | 'https'; url: string }> {
  const host = formatHost(address)
  const order: Array<'http' | 'https'> = pageProtocol === 'https:' ? ['https', 'http'] : ['http', 'https']
  return order.map((scheme) => ({ scheme, url: `${scheme}://${host}/` }))
}

function isSecurityException(error: unknown): boolean {
  if (typeof DOMException !== 'undefined' && error instanceof DOMException) {
    return error.name === 'SecurityError' || error.name === 'NotAllowedError'
  }
  return error instanceof Error && (error.name === 'SecurityError' || error.name === 'NotAllowedError')
}

function browserFetch(): typeof fetch {
  if (typeof fetch !== 'function') throw new TypeError('fetch 不可用')
  return fetch
}

function securePublicPageToPrivateTarget(page: BrowserPageContext, target: NetworkProbeTarget): boolean {
  return page.protocol === 'https:' && !isPrivateHost(page.hostname) && isPrivateHost(target.address)
}

/**
 * 单目标探测：浏览器只使用 HTTP/HTTPS，不读取响应体，也不根据 IP 子网猜测。
 * no-cors 的不透明响应无法读取状态码；收到 HTTP 响应即视为“明确可达”。
 * 若请求被 mixed content / Private Network Access / CORS 等浏览器策略挡住，
 * 或异常无法与网络失败区分，则返回 unknown，绝不当作 WAN。
 */
export async function browserProbeTarget(
  target: NetworkProbeTarget,
  timeoutMs: number,
  outerSignal?: AbortSignal,
): Promise<ProbeOutcome> {
  const page = currentPage()
  const urls = buildProbeURLs(target.address, page.protocol)
  let sawUnknown = false
  const deadline = Date.now() + Math.max(1, timeoutMs)

  for (const item of urls) {
    const remaining = deadline - Date.now()
    if (remaining <= 0 || outerSignal?.aborted) return sawUnknown ? 'unknown' : 'failure'
    // 安全页面请求 HTTP 目标会被 mixed content 拦截；继续尝试 HTTPS，最终仍无法确定时为 UNKNOWN。
    if (page.protocol === 'https:' && item.scheme === 'http') {
      sawUnknown = true
      continue
    }

    let timedOut = false
    const controller = new AbortController()
    const timer = setTimeout(() => {
      timedOut = true
      controller.abort()
    }, remaining)
    const onOuterAbort = () => controller.abort()
    outerSignal?.addEventListener('abort', onOuterAbort, { once: true })
    try {
      await browserFetch()(item.url, {
        method: 'GET',
        mode: 'no-cors',
        cache: 'no-store',
        credentials: 'omit',
        referrerPolicy: 'no-referrer',
        signal: controller.signal,
      })
      if (controller.signal.aborted && !timedOut && !outerSignal?.aborted) return 'unknown'
      return 'success'
    } catch (error) {
      if (timedOut || outerSignal?.aborted) return sawUnknown ? 'unknown' : 'failure'
      if (isSecurityException(error)) {
        sawUnknown = true
        continue
      }
      // HTTPS 页面到私网目标可能被 PNA 拦截，类型错误无法与真实连接失败区分，按 UNKNOWN 处理。
      if (securePublicPageToPrivateTarget(page, target)) {
        sawUnknown = true
        continue
      }
      if (page.protocol === 'https:' && item.scheme === 'http') {
        sawUnknown = true
        continue
      }
    } finally {
      clearTimeout(timer)
      outerSignal?.removeEventListener('abort', onOuterAbort)
    }
  }
  return sawUnknown ? 'unknown' : 'failure'
}

async function runRound(
  targets: NetworkProbeTarget[],
  timeoutMs: number,
  probe: ProbeFn,
  signal?: AbortSignal,
): Promise<RoundOutcome> {
  if (targets.length === 0) return { outcome: 'unknown' }
  if (signal?.aborted) return { outcome: 'unknown' }
  const controller = new AbortController()
  const onOuterAbort = () => controller.abort()
  signal?.addEventListener('abort', onOuterAbort, { once: true })

  return new Promise<RoundOutcome>((resolve) => {
    let finished = false
    let settled = 0
    let sawUnknown = false
    const finish = (outcome: ProbeOutcome) => {
      if (finished) return
      finished = true
      controller.abort()
      resolve({ outcome })
    }

    targets.forEach((target) => {
      Promise.resolve()
        .then(() => probe(target, timeoutMs, controller.signal))
        .then((outcome) => {
          if (outcome === 'success') {
            finish('success')
            return
          }
          if (outcome === 'unknown') sawUnknown = true
        })
        .catch(() => {
          // 单个目标的异常不能影响其他目标；全部异常按明确失败处理。
        })
        .finally(() => {
          settled += 1
          if (settled === targets.length) finish(sawUnknown ? 'unknown' : 'failure')
        })
    })
  }).finally(() => {
    signal?.removeEventListener('abort', onOuterAbort)
  })
}

function result(
  status: AutoNetworkStatus,
  reason: AutoDetectionReason,
  rounds: number,
  startedAt: number,
  now: () => Date,
): AutoDetectionResult {
  const finishedAt = now()
  return {
    status,
    reason,
    rounds,
    checkedAt: finishedAt.toISOString(),
    duration_ms: Math.max(0, finishedAt.getTime() - startedAt),
  }
}

/**
 * 两轮判断：首轮任一点成功立即 LAN；全部明确失败才复核；复核全失败才 WAN。
 * 任一轮出现浏览器安全限制导致的 UNKNOWN，直接保留 UNKNOWN，不降级为 WAN。
 */
export async function detectNetwork(
  targets: NetworkProbeTarget[],
  options: DetectionOptions = {},
): Promise<AutoDetectionResult> {
  const probe = options.probe ?? browserProbeTarget
  const now = options.now ?? (() => new Date())
  const timeouts = options.timeouts ?? [AUTO_ROUND_1_TIMEOUT_MS, AUTO_ROUND_2_TIMEOUT_MS]
  const startedAt = now()
  const startedMs = startedAt.getTime()
  if (targets.length === 0) {
    return {
      status: 'unknown',
      reason: 'no_targets',
      rounds: 0,
      checkedAt: startedAt.toISOString(),
      duration_ms: 0,
    }
  }
  if (options.signal?.aborted) {
    return {
      status: 'unknown',
      reason: 'browser_restricted',
      rounds: 0,
      checkedAt: startedAt.toISOString(),
      duration_ms: 0,
    }
  }

  const round1 = await runRound(targets, timeouts[0], probe, options.signal)
  if (round1.outcome === 'success') return result('lan', 'matched', 1, startedMs, now)
  if (round1.outcome === 'unknown') return result('unknown', 'browser_restricted', 1, startedMs, now)
  if (options.signal?.aborted) return result('unknown', 'browser_restricted', 1, startedMs, now)

  const round2 = await runRound(targets, timeouts[1], probe, options.signal)
  if (round2.outcome === 'success') return result('lan', 'matched', 2, startedMs, now)
  if (round2.outcome === 'unknown') return result('unknown', 'browser_restricted', 2, startedMs, now)
  return result('wan', 'all_failed', 2, startedMs, now)
}

/** 浏览器自动判断协调器：缓存、最小间隔、单航班和离线失效都在这里。 */
export function createAutoDetector(options: AutoDetectorOptions) {
  const timeouts = options.timeouts ?? [AUTO_ROUND_1_TIMEOUT_MS, AUTO_ROUND_2_TIMEOUT_MS]
  const minIntervalMs = options.minIntervalMs ?? AUTO_MIN_INTERVAL_MS
  const now = options.now ?? (() => Date.now())
  const page = options.page ?? currentPage
  const isOnline = options.isOnline ?? (() => (typeof navigator === 'undefined' ? true : navigator.onLine !== false))
  const probe = options.probe ?? browserProbeTarget
  const listeners = new Set<() => void>()
  let snapshot: AutoSnapshot = { phase: 'idle', result: null }
  let inFlight: Promise<void> | null = null
  let activeController: AbortController | null = null
  let lastStart = 0

  const emit = (next: AutoSnapshot) => {
    snapshot = next
    listeners.forEach((listener) => listener())
  }
  const subscribe = (listener: () => void) => {
    listeners.add(listener)
    return () => listeners.delete(listener)
  }
  const getSnapshot = () => snapshot

  const finish = (resultValue: AutoDetectionResult) => {
    emit({ phase: 'checked', result: resultValue })
  }

  const run = (force = false): Promise<void> => {
    if (inFlight) return inFlight
    const started = now()
    if (!force && lastStart > 0 && started - lastStart < minIntervalMs) return Promise.resolve()
    lastStart = started

    const controller = new AbortController()
    activeController = controller
    emit({ phase: 'probing', result: snapshot.result })

    const execute = async () => {
      const startedAt = new Date(now())
      try {
        if (!isOnline()) {
          finish({
            status: 'unknown',
            reason: 'offline',
            rounds: 0,
            checkedAt: new Date(now()).toISOString(),
            duration_ms: 0,
          })
          return
        }
        const payload = await options.loadTargets(controller.signal)
        if (controller.signal.aborted) return
        const detected = await detectNetwork(payload.targets, {
          probe,
          signal: controller.signal,
          timeouts,
          now: options.now ? () => new Date(now()) : undefined,
        })
        if (controller.signal.aborted) return
        const elapsed = Math.max(0, now() - startedAt.getTime())
        finish({ ...detected, duration_ms: elapsed })
      } catch {
        if (controller.signal.aborted) return
        finish({
          status: 'unknown',
          reason: 'targets_unavailable',
          rounds: 0,
          checkedAt: new Date(now()).toISOString(),
          duration_ms: Math.max(0, now() - startedAt.getTime()),
        })
      }
    }

    let task: Promise<void>
    task = execute().finally(() => {
      if (inFlight === task) inFlight = null
      if (activeController === controller) activeController = null
    })
    inFlight = task
    return task
  }

  const invalidate = (reason: AutoDetectionReason = 'browser_restricted') => {
    activeController?.abort()
    activeController = null
    inFlight = null
    lastStart = 0
    finish({
      status: 'unknown',
      reason,
      rounds: 0,
      checkedAt: new Date(now()).toISOString(),
      duration_ms: 0,
    })
  }

  return {
    subscribe,
    getSnapshot,
    run,
    invalidate,
    page,
    current: () => snapshot.result,
  }
}
