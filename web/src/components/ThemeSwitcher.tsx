import type { ThemeMode } from '../types'
import { nextTheme, themeCopy, themeOrder } from '../lib'
import { AutoIcon, MoonIcon, SunIcon } from './Icons'

const themeIcons = {
  auto: AutoIcon,
  light: SunIcon,
  dark: MoonIcon,
}

export function ThemeSwitcher({
  mode,
  onChange,
  compact = false,
}: {
  mode: ThemeMode
  onChange: (mode: ThemeMode) => void
  compact?: boolean
}) {
  const CurrentIcon = themeIcons[mode]

  if (compact) {
    return (
      <button
        type="button"
        className="icon-button theme-compact"
        aria-label={`切换外观，当前${themeCopy[mode].label}`}
        title={`外观：${themeCopy[mode].label}`}
        onClick={() => onChange(nextTheme(mode))}
      >
        <CurrentIcon size={18} />
      </button>
    )
  }

  return (
    <div className="theme-switch" role="group" aria-label="外观模式">
      {themeOrder.map((value) => {
        const Icon = themeIcons[value]
        return (
          <button
            key={value}
            type="button"
            className="theme-option"
            aria-pressed={mode === value}
            title={themeCopy[value].hint}
            onClick={() => onChange(value)}
          >
            <Icon size={16} />
            <span>{themeCopy[value].label}</span>
          </button>
        )
      })}
    </div>
  )
}
