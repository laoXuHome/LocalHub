import { describe, expect, it } from 'vitest'
import {
  bookmarkHref,
  bookmarkTarget,
  cardSizeFor,
  hueFor,
  iconKind,
  iconSource,
  iconText,
  initialLetter,
  isHTTPURL,
  isThemeMode,
  nextTheme,
  themeFor,
} from './lib'
import type { Bookmark } from './types'

function bookmark(overrides: Partial<Bookmark> = {}): Bookmark {
  return {
    id: 1,
    name: '示例书签',
    lan_url: 'http://192.168.1.10:8080',
    wan_url: 'https://example.com',
    description: '',
    icon_type: '',
    icon_value: '',
    enabled: true,
    visibility: 'public',
    effective_visibility: 'public',
    open_mode: 'new_tab',
    folder_path: [],
    sort_order: 1,
    has_icon: false,
    ...overrides,
    ...overrides,
  }
}

describe('theme helpers', () => {
  it('识别三种合法主题值', () => {
    expect(isThemeMode('auto')).toBe(true)
    expect(isThemeMode('light')).toBe(true)
    expect(isThemeMode('dark')).toBe(true)
    expect(isThemeMode('system')).toBe(false)
    expect(isThemeMode(null)).toBe(false)
  })

  it('按 auto → light → dark 循环切换', () => {
    expect(nextTheme('auto')).toBe('light')
    expect(nextTheme('light')).toBe('dark')
    expect(nextTheme('dark')).toBe('auto')
  })

  it('auto 跟随系统，明确模式忽略系统', () => {
    expect(themeFor('auto', true)).toBe('dark')
    expect(themeFor('auto', false)).toBe('light')
    expect(themeFor('light', true)).toBe('light')
    expect(themeFor('dark', false)).toBe('dark')
  })
})

describe('URL helpers', () => {
  it('只接受 http 与 https', () => {
    expect(isHTTPURL('https://example.com')).toBe(true)
    expect(isHTTPURL('http://192.168.1.2:3000/path')).toBe(true)
    expect(isHTTPURL('ftp://example.com')).toBe(false)
    expect(isHTTPURL('javascript:alert(1)')).toBe(false)
    expect(isHTTPURL('example.com')).toBe(false)
    expect(isHTTPURL('')).toBe(false)
  })

  it('优先使用合法 WAN 地址', () => {
    expect(bookmarkHref(bookmark())).toBe('https://example.com')
  })

  it('WAN 不可用时回退到合法 LAN 地址', () => {
    const item = bookmark({ wan_url: 'ftp://example.com', lan_url: 'http://10.0.0.2:8080' })
    expect(bookmarkHref(item)).toBe('http://10.0.0.2:8080')
  })

  it('地址两段都非法时返回空字符串', () => {
    expect(bookmarkHref(bookmark({ wan_url: 'not-a-url', lan_url: '  ' }))).toBe('')
  })

  it('去除地址两端空白', () => {
    expect(bookmarkHref(bookmark({ wan_url: '  https://example.com/a  ' }))).toBe('https://example.com/a')
  })

  it('手动 LAN 选择 LAN 地址，手动 WAN 选择 WAN 地址', () => {
    expect(bookmarkHref(bookmark(), 'lan')).toBe('http://192.168.1.10:8080')
    expect(bookmarkHref(bookmark(), 'wan')).toBe('https://example.com')
  })

  it('默认模式沿用原有 WAN 优先、LAN 回退规则', () => {
    expect(bookmarkHref(bookmark(), 'default')).toBe('https://example.com')
    expect(bookmarkHref(bookmark({ wan_url: '', lan_url: 'http://10.0.0.2:8080' }), 'default')).toBe('http://10.0.0.2:8080')
  })

  it('对应地址缺失或非法时沿用原有默认规则，不编造地址', () => {
    expect(bookmarkHref(bookmark({ lan_url: '' }), 'lan')).toBe('https://example.com')
    expect(bookmarkHref(bookmark({ lan_url: 'not-a-url' }), 'lan')).toBe('https://example.com')
    expect(bookmarkHref(bookmark({ wan_url: 'ftp://example.com' }), 'wan')).toBe('http://192.168.1.10:8080')
    expect(bookmarkHref(bookmark({ wan_url: '', lan_url: '  ' }), 'lan')).toBe('')
    expect(bookmarkHref(bookmark({ wan_url: '', lan_url: '' }), 'wan')).toBe('')
  })

  it('去除所选地址两端空白且不拼接路径', () => {
    expect(bookmarkHref(bookmark({ lan_url: '  http://10.0.0.2:8080/a  ' }), 'lan')).toBe('http://10.0.0.2:8080/a')
    expect(bookmarkHref(bookmark({ wan_url: '  https://example.com/a  ' }), 'wan')).toBe('https://example.com/a')
  })

  it('self 与 current_tab 在当前页打开，其余新页打开', () => {
    expect(bookmarkTarget(bookmark({ open_mode: 'self' }))).toBe('_self')
    expect(bookmarkTarget(bookmark({ open_mode: 'current_tab' }))).toBe('_self')
    expect(bookmarkTarget(bookmark({ open_mode: 'new_tab' }))).toBe('_blank')
    expect(bookmarkTarget(bookmark({ open_mode: '' }))).toBe('_blank')
    expect(bookmarkTarget(bookmark({ open_mode: 'external' }))).toBe('_blank')
  })
})

describe('icon helpers', () => {
  it('已迁移文件图标使用 icon_url', () => {
    const item = bookmark({ icon_type: 'file', has_icon: true, icon_url: '/api/bookmarks/1/icon' })
    expect(iconKind(item)).toBe('image')
    expect(iconSource(item)).toBe('/api/bookmarks/1/icon')
  })

  it('文件图标缺失时回退首字，不猜测地址', () => {
    const item = bookmark({ icon_type: 'file', has_icon: false, icon_url: undefined })
    expect(iconKind(item)).toBe('fallback')
    expect(iconSource(item)).toBe('')
  })

  it('合法 favicon URL 作为图片', () => {
    const item = bookmark({ icon_type: 'favicon', icon_value: 'https://example.com/favicon.ico' })
    expect(iconKind(item)).toBe('image')
    expect(iconSource(item)).toBe('https://example.com/favicon.ico')
  })

  it('非法图片地址不渲染为图片', () => {
    const item = bookmark({ icon_type: 'url', icon_value: 'javascript:alert(1)' })
    expect(iconKind(item)).toBe('fallback')
    expect(iconSource(item)).toBe('')
  })

  it('emoji 与文字图标使用字形', () => {
    expect(iconKind(bookmark({ icon_type: 'emoji', icon_value: '🏠' }))).toBe('glyph')
    expect(iconKind(bookmark({ icon_type: 'text', icon_value: 'LH' }))).toBe('glyph')
    expect(iconText(bookmark({ icon_type: 'emoji', icon_value: ' 🏠 ' }))).toBe('🏠')
    expect(iconText(bookmark({ icon_type: 'text', icon_value: ' LH ' }))).toBe('LH')
  })

  it('空白文字图标回退首字', () => {
    const item = bookmark({ icon_type: 'text', icon_value: '   ' })
    expect(iconKind(item)).toBe('fallback')
    expect(iconText(item)).toBe('')
  })

  it('首字支持中文、英文与空名称', () => {
    expect(initialLetter('网易云音乐')).toBe('网')
    expect(initialLetter('nas')).toBe('N')
    expect(initialLetter('')).toBe('·')
    expect(initialLetter('   ')).toBe('·')
  })
})

describe('layout helpers', () => {
  it('首张为大卡，2 至 5 张为中卡，其余为小卡', () => {
    expect(cardSizeFor(0)).toBe('large')
    expect(cardSizeFor(1)).toBe('medium')
    expect(cardSizeFor(4)).toBe('medium')
    expect(cardSizeFor(5)).toBe('small')
    expect(cardSizeFor(99)).toBe('small')
  })

  it('色相只由书签 id 决定且稳定', () => {
    expect(hueFor(1)).toBe(246)
    expect(hueFor(7)).toBe(224)
    expect(hueFor(14)).toBe(224)
  })
})
