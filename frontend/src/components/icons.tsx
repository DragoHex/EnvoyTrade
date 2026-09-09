// Material Design icon glyphs (Apache-2.0, Google) reused as inline SVG so
// buttons render consistently across platforms instead of relying on emoji
// font coverage.

export function BlockIcon() {
  return (
    <svg viewBox="0 0 24 24" fill="currentColor">
      <path d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2ZM4 12c0-4.42 3.58-8 8-8 1.85 0 3.55.63 4.9 1.69L5.69 16.9C4.63 15.55 4 13.85 4 12Zm8 8c-1.85 0-3.55-.63-4.9-1.69L18.31 7.1C19.37 8.45 20 10.15 20 12c0 4.42-3.58 8-8 8Z" />
    </svg>
  )
}

export function PlayCircleIcon() {
  return (
    <svg viewBox="0 0 24 24" fill="currentColor">
      <path d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2Zm-2 14.5v-9l6 4.5-6 4.5Z" />
    </svg>
  )
}

export function SyncIcon(props: { spinning?: boolean }) {
  return (
    <svg viewBox="0 0 24 24" fill="currentColor" classList={{ 'icon-spin': props.spinning }}>
      <path d="M12 4V1L8 5l4 4V6c3.31 0 6 2.69 6 6 0 1.01-.25 1.97-.7 2.8l1.46 1.46C19.54 15.03 20 13.57 20 12c0-4.42-3.58-8-8-8Zm0 14c-3.31 0-6-2.69-6-6 0-1.01.25-1.97.7-2.8L5.24 7.74C4.46 8.97 4 10.43 4 12c0 4.42 3.58 8 8 8v3l4-4-4-4v3Z" />
    </svg>
  )
}

export function CropSquareIcon() {
  return (
    <svg viewBox="0 0 24 24" fill="currentColor">
      <path d="M18 4H6c-1.1 0-2 .9-2 2v12c0 1.1.9 2 2 2h12c1.1 0 2-.9 2-2V6c0-1.1-.9-2-2-2Zm0 14H6V6h12v12Z" />
    </svg>
  )
}

export function LogoutIcon() {
  return (
    <svg viewBox="0 0 24 24" fill="currentColor">
      <path d="M17 7l-1.41 1.41L17.17 10H9v2h8.17l-1.58 1.59L17 15l4-4-4-4ZM5 5h7V3H5c-1.1 0-2 .9-2 2v14c0 1.1.9 2 2 2h7v-2H5V5Z" />
    </svg>
  )
}
