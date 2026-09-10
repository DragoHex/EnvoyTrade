import { createEffect, createSignal, type JSX } from 'solid-js'

export type Theme = 'light' | 'dark'

const STORAGE_KEY = 'theme'

function initialTheme(): Theme {
  const stored = sessionStorage.getItem(STORAGE_KEY)
  if (stored === 'light' || stored === 'dark') return stored
  if (typeof window !== 'undefined' && typeof window.matchMedia === 'function') {
    return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'
  }
  return 'light'
}

const [theme, setTheme] = createSignal<Theme>(initialTheme())

export function useTheme() {
  return {
    theme,
    toggleTheme: () =>
      setTheme((t) => {
        const next = t === 'light' ? 'dark' : 'light'
        sessionStorage.setItem(STORAGE_KEY, next)
        return next
      }),
  }
}

// data-theme is set on <html> (not a wrapper div) so the CSS variables it
// selects on are visible to every element, including <body> — a var
// defined lower in the tree than the element using it never resolves.
export function ThemeProvider(props: { children: JSX.Element }) {
  createEffect(() => {
    document.documentElement.dataset.theme = theme()
  })
  return <>{props.children}</>
}
