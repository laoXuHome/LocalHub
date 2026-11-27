import { useId, type ReactNode } from 'react'

type IconProps = { size?: number; className?: string }

function Svg({ size = 20, className, children }: IconProps & { children: ReactNode }) {
  return (
    <svg
      className={className}
      width={size}
      height={size}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.8"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      {children}
    </svg>
  )
}

export function HomeIcon(props: IconProps) {
  return (
    <Svg {...props}>
      <path d="M3 10.5 12 3l9 7.5" />
      <path d="M5.5 9.5V21h13V9.5" />
      <path d="M9.5 21v-6h5v6" />
    </Svg>
  )
}

export function BookmarkIcon(props: IconProps) {
  return (
    <Svg {...props}>
      <path d="M6 3.5h12v17l-6-4.2-6 4.2z" />
    </Svg>
  )
}

export function ServerIcon(props: IconProps) {
  return (
    <Svg {...props}>
      <rect x="3" y="4" width="18" height="6" rx="2" />
      <rect x="3" y="14" width="18" height="6" rx="2" />
      <path d="M7 7h.01M7 17h.01" />
    </Svg>
  )
}

export function SearchIcon(props: IconProps) {
  return (
    <Svg {...props}>
      <circle cx="10.5" cy="10.5" r="6.5" />
      <path d="m15.5 15.5 4 4" />
    </Svg>
  )
}

export function UserIcon(props: IconProps) {
  return (
    <Svg {...props}>
      <circle cx="12" cy="8" r="4" />
      <path d="M4.5 21c.6-4.2 3.4-6.5 7.5-6.5s6.9 2.3 7.5 6.5" />
    </Svg>
  )
}

export function ChevronLeftIcon(props: IconProps) {
  return (
    <Svg {...props}>
      <path d="m14.5 5-7 7 7 7" />
    </Svg>
  )
}

export function ChevronRightIcon(props: IconProps) {
  return (
    <Svg {...props}>
      <path d="m9.5 5 7 7-7 7" />
    </Svg>
  )
}

export function SunIcon(props: IconProps) {
  return (
    <Svg {...props}>
      <circle cx="12" cy="12" r="4" />
      <path d="M12 2v2M12 20v2M4.93 4.93l1.42 1.42M17.65 17.65l1.42 1.42M2 12h2M20 12h2M4.93 19.07l1.42-1.42M17.65 6.35l1.42-1.42" />
    </Svg>
  )
}

export function MoonIcon(props: IconProps) {
  return (
    <Svg {...props}>
      <path d="M20 15.2A8.2 8.2 0 0 1 8.8 4 8.4 8.4 0 1 0 11.2 20 8.2 8.2 0 0 0 8.8-4.8z" />
    </Svg>
  )
}

export function AutoIcon(props: IconProps) {
  return (
    <Svg {...props}>
      <circle cx="12" cy="12" r="8.5" />
      <path d="M12 3.5v17" />
      <path d="M12 3.5c4.7 0 8.5 3.8 8.5 8.5s-3.8 8.5-8.5 8.5" fill="currentColor" fillOpacity=".18" />
    </Svg>
  )
}

export function RefreshIcon(props: IconProps) {
  return (
    <Svg {...props}>
      <path d="M20 11a8 8 0 1 0-2.3 5.7" />
      <path d="M20 5v6h-6" />
    </Svg>
  )
}

export function WifiOffIcon(props: IconProps) {
  return (
    <Svg {...props}>
      <path d="M3 3l18 18" />
      <path d="M9.5 9.5A12 12 0 0 1 21 10" />
      <path d="M5.5 6.5A17 17 0 0 0 3 8" />
      <path d="M8.5 13.5a7 7 0 0 1 7 1" />
      <path d="M12 18h.01" />
    </Svg>
  )
}

export function FolderIcon(props: IconProps) {
  return (
    <Svg {...props}>
      <path d="M3 7.5A2.5 2.5 0 0 1 5.5 5h4l2 2H19a2 2 0 0 1 2 2v8.5a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z" />
    </Svg>
  )
}

export function CloseIcon(props: IconProps) {
  return (
    <Svg {...props}>
      <path d="M6 6l12 12M18 6l-12 12" />
    </Svg>
  )
}

export function KeyIcon(props: IconProps) {
  return (
    <Svg {...props}>
      <circle cx="8" cy="15" r="4" />
      <path d="m11 12 9-9M17 4l3 3M14.5 6.5l2 2" />
    </Svg>
  )
}

export function LogoutIcon(props: IconProps) {
  return (
    <Svg {...props}>
      <path d="M10 4H5.5A1.5 1.5 0 0 0 4 5.5v13A1.5 1.5 0 0 0 5.5 20H10" />
      <path d="m14 8 4 4-4 4M9 12h9" />
    </Svg>
  )
}

export function ShieldIcon(props: IconProps) {
  return (
    <Svg {...props}>
      <path d="M12 3 5 6v5.5c0 4.2 2.8 7.7 7 9.5 4.2-1.8 7-5.3 7-9.5V6z" />
      <path d="m9 12 2 2 4-4" />
    </Svg>
  )
}

export function PencilIcon(props: IconProps) {
  return (
    <Svg {...props}>
      <path d="m4 20 4.2-1 11-11a2.4 2.4 0 0 0-3.2-3.2l-11 11L4 20Z" />
      <path d="m14.5 6.5 3 3" />
    </Svg>
  )
}

export function PlusIcon(props: IconProps) {
  return (
    <Svg {...props}>
      <path d="M12 5v14M5 12h14" />
    </Svg>
  )
}

export function UndoIcon(props: IconProps) {
  return (
    <Svg {...props}>
      <path d="M9 7H5v-4" />
      <path d="M5.5 7.5A7.5 7.5 0 1 1 5 14" />
    </Svg>
  )
}

export function RedoIcon(props: IconProps) {
  return (
    <Svg {...props}>
      <path d="M15 7h4v-4" />
      <path d="M18.5 7.5A7.5 7.5 0 1 0 19 14" />
    </Svg>
  )
}

export function EyeIcon(props: IconProps) {
  return (
    <Svg {...props}>
      <path d="M2.5 12s3.5-6 9.5-6 9.5 6 9.5 6-3.5 6-9.5 6-9.5-6-9.5-6Z" />
      <circle cx="12" cy="12" r="2.8" />
    </Svg>
  )
}

export function SaveIcon(props: IconProps) {
  return (
    <Svg {...props}>
      <path d="M5 3.5h11l3 3V21H5z" />
      <path d="M8 3.5v6h8v-6M8 21v-7h8v7" />
    </Svg>
  )
}

export function TrashIcon(props: IconProps) {
  return (
    <Svg {...props}>
      <path d="M4 7h16M9 7V4h6v3M6.5 7l1 14h11l1-14" />
      <path d="M10 11v6M14 11v6" />
    </Svg>
  )
}

export function BrandMark({ size = 36 }: { size?: number }) {
  const rawId = useId()
  const gradientId = `brand-${rawId.replace(/[^a-zA-Z0-9_-]/g, '')}`
  return (
    <svg width={size} height={size} viewBox="0 0 40 40" aria-hidden="true" className="brand-mark">
      <defs>
        <linearGradient id={gradientId} x1="4" y1="4" x2="36" y2="36">
          <stop offset="0" stopColor="#7c8cff" />
          <stop offset="1" stopColor="#9b5cf6" />
        </linearGradient>
      </defs>
      <rect x="3" y="3" width="34" height="34" rx="11" fill={`url(#${gradientId})`} />
      <path d="M12 14.5h16M12 20h10M12 25.5h13" stroke="white" strokeOpacity=".92" strokeWidth="2" strokeLinecap="round" />
      <circle cx="28.5" cy="25.5" r="2.1" fill="white" fillOpacity=".92" />
    </svg>
  )
}
