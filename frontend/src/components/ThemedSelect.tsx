import { createEffect, createSignal, createUniqueId, For, onCleanup, Show, type JSX } from 'solid-js'

export interface ThemedSelectBadge {
  text: string
  variant?: 'emerald' | 'slate' | 'amber' | 'blue'
}

export interface ThemedSelectOption<T extends string = string> {
  value: T
  label: string
  description?: string
  badge?: ThemedSelectBadge
  icon?: (props: { class?: string }) => JSX.Element
  disabled?: boolean
}

export interface ThemedSelectProps<T extends string = string> {
  id?: string
  name?: string
  value: T
  options: ThemedSelectOption<T>[]
  onChange: (val: T) => void
  disabled?: boolean
  required?: boolean
  placeholder?: string
  'aria-label'?: string
  class?: string
  style?: JSX.CSSProperties | Record<string, string | number>
  prefix?: JSX.Element
}

function getBadgeStyle(variant?: 'emerald' | 'slate' | 'amber' | 'blue'): JSX.CSSProperties {
  switch (variant) {
    case 'emerald':
      return {
        background: 'rgba(4, 180, 136, 0.15)',
        color: 'var(--color-accent, #04b488)',
        border: '1px solid rgba(4, 180, 136, 0.3)',
      }
    case 'amber':
      return {
        background: 'rgba(245, 158, 11, 0.15)',
        color: '#f59e0b',
        border: '1px solid rgba(245, 158, 11, 0.3)',
      }
    case 'blue':
      return {
        background: 'rgba(59, 130, 246, 0.15)',
        color: '#3b82f6',
        border: '1px solid rgba(59, 130, 246, 0.3)',
      }
    case 'slate':
    default:
      return {
        background: 'rgba(100, 116, 139, 0.15)',
        color: 'var(--color-text-muted, #94a3b8)',
        border: '1px solid rgba(100, 116, 139, 0.25)',
      }
  }
}

export function ThemedSelect<T extends string = string>(props: ThemedSelectProps<T>) {
  const [isOpen, setIsOpen] = createSignal(false)
  const generatedId = createUniqueId()
  const selectId = () => props.id || generatedId
  let containerRef: HTMLDivElement | undefined

  createEffect(() => {
    if (!isOpen()) return

    const handlePointerDown = (e: MouseEvent) => {
      const target = e.target as Node | null
      if (!target || containerRef?.contains(target)) return
      setIsOpen(false)
    }

    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        setIsOpen(false)
      }
    }

    document.addEventListener('mousedown', handlePointerDown)
    document.addEventListener('keydown', handleKeyDown)

    onCleanup(() => {
      document.removeEventListener('mousedown', handlePointerDown)
      document.removeEventListener('keydown', handleKeyDown)
    })
  })

  let selectRef: HTMLSelectElement | undefined

  createEffect(() => {
    // Keep native select value in sync when options or value update dynamically
    const val = props.value
    void props.options
    if (selectRef && val !== undefined) {
      selectRef.value = val
    }
  })

  const selectedOption = () => props.options.find((opt) => opt.value === props.value)
  const isPlaceholder = () => !props.value || !selectedOption() || !!selectedOption()?.disabled

  return (
    <div
      ref={containerRef}
      class={`themed-select-container ${props.class || ''}`}
      style={{
        position: 'relative',
        width: '100%',
        'box-sizing': 'border-box',
        ...(typeof props.style === 'object' ? props.style : {}),
      }}
    >
      {/* Synchronized native select for programmatic testing & accessibility */}
      <select
        ref={selectRef}
        id={selectId()}
        name={props.name}
        aria-label={props['aria-label']}
        value={props.value}
        disabled={props.disabled}
        required={props.required}
        tabIndex={-1}
        onChange={(e) => {
          props.onChange(e.currentTarget.value as T)
        }}
        style={{
          position: 'absolute',
          top: 0,
          left: 0,
          width: '100%',
          height: '100%',
          opacity: 0,
          'pointer-events': 'none',
          'z-index': -1,
        }}
      >
        <For each={props.options}>
          {(opt) => (
            <option value={opt.value} disabled={opt.disabled} selected={opt.value === props.value}>
              {opt.label}
            </option>
          )}
        </For>
      </select>

      {/* Custom styled trigger button */}
      <div
        role="button"
        aria-haspopup="listbox"
        aria-expanded={isOpen()}
        aria-disabled={props.disabled}
        tabIndex={props.disabled ? -1 : 0}
        onClick={(e) => {
          e.stopPropagation()
          if (props.disabled) return
          setIsOpen(!isOpen())
        }}
        onKeyDown={(e) => {
          if (props.disabled) return
          if (e.key === 'Enter' || e.key === ' ') {
            e.preventDefault()
            e.stopPropagation()
            setIsOpen(!isOpen())
          }
        }}
        style={{
          display: 'flex',
          'align-items': 'center',
          'justify-content': 'space-between',
          padding: '0.5rem 0.75rem',
          'border-radius': '6px',
          border: isOpen() ? '1px solid var(--color-accent, #04b488)' : '1px solid var(--color-border, #2a2c34)',
          background: 'var(--color-bg, #0f1117)',
          color: 'var(--color-text, #f0f0f2)',
          'font-size': '0.9rem',
          cursor: props.disabled ? 'not-allowed' : 'pointer',
          opacity: props.disabled ? 0.6 : 1,
          'user-select': 'none',
          transition: 'border-color 0.15s ease, box-shadow 0.15s ease',
          width: '100%',
          'box-sizing': 'border-box',
          outline: 'none',
        }}
      >
        <div
          style={{
            display: 'flex',
            'align-items': 'center',
            gap: '0.5rem',
            overflow: 'hidden',
            'text-overflow': 'ellipsis',
            'white-space': 'nowrap',
            flex: 1,
          }}
        >
          <Show when={props.prefix}>{props.prefix}</Show>
          <span
            style={{
              overflow: 'hidden',
              'text-overflow': 'ellipsis',
              'white-space': 'nowrap',
              color: isPlaceholder() ? 'var(--color-text-secondary, #9da3ae)' : 'var(--color-text, #f0f0f2)',
            }}
          >
            {selectedOption()?.label || props.placeholder || props.value}
          </span>
          <Show when={selectedOption()?.badge}>
            <span
              style={{
                'font-size': '0.7rem',
                padding: '0.12rem 0.45rem',
                'border-radius': '4px',
                'font-weight': 600,
                'flex-shrink': 0,
                ...getBadgeStyle(selectedOption()!.badge!.variant),
              }}
            >
              {selectedOption()!.badge!.text}
            </span>
          </Show>
        </div>

        {/* Chevron icon indicator */}
        <svg
          width="14"
          height="14"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          stroke-width="2"
          stroke-linecap="round"
          stroke-linejoin="round"
          style={{
            transform: isOpen() ? 'rotate(180deg)' : 'rotate(0deg)',
            transition: 'transform 0.15s ease',
            color: 'var(--color-text-muted, #94a3b8)',
            'flex-shrink': 0,
            'margin-left': '0.5rem',
          }}
        >
          <polyline points="6 9 12 15 18 9" />
        </svg>
      </div>

      {/* Custom styled popover menu */}
      <Show when={isOpen() && !props.disabled}>
        <div
          role="listbox"
          aria-label={props['aria-label'] || 'Options'}
          onMouseDown={(e) => {
            e.preventDefault()
            e.stopPropagation()
          }}
          onClick={(e) => e.stopPropagation()}
          style={{
            position: 'absolute',
            top: 'calc(100% + 4px)',
            left: 0,
            right: 0,
            'z-index': 50,
            background: 'var(--color-surface, #1a1c24)',
            border: '1px solid var(--color-border, #2a2c34)',
            'border-radius': '8px',
            'box-shadow': '0 10px 25px -5px rgba(0, 0, 0, 0.3), 0 8px 10px -6px rgba(0, 0, 0, 0.2)',
            'max-height': '240px',
            'overflow-y': 'auto',
            padding: '0.35rem',
          }}
        >
          <For each={props.options}>
            {(opt) => {
              const isSelected = () => opt.value === props.value
              return (
                <div
                  role="option"
                  aria-selected={isSelected()}
                  aria-disabled={opt.disabled}
                  onMouseDown={(e) => {
                    e.preventDefault()
                    e.stopPropagation()
                  }}
                  onClick={(e) => {
                    e.preventDefault()
                    e.stopPropagation()
                    if (opt.disabled) return
                    props.onChange(opt.value)
                    setIsOpen(false)
                  }}
                  style={{
                    display: 'flex',
                    'align-items': 'center',
                    'justify-content': 'space-between',
                    padding: '0.5rem 0.65rem',
                    'border-radius': '6px',
                    cursor: opt.disabled ? 'not-allowed' : 'pointer',
                    opacity: opt.disabled ? 0.45 : 1,
                    'font-size': '0.85rem',
                    background: isSelected() ? 'rgba(4, 180, 136, 0.12)' : 'transparent',
                    color: isSelected() ? 'var(--color-accent, #04b488)' : 'var(--color-text, #f0f0f2)',
                    'font-weight': isSelected() ? 600 : 400,
                    transition: 'background 0.12s ease',
                    'margin-bottom': '2px',
                  }}
                  onMouseEnter={(e) => {
                    if (!isSelected() && !opt.disabled) e.currentTarget.style.background = 'rgba(255, 255, 255, 0.05)'
                  }}
                  onMouseLeave={(e) => {
                    if (!isSelected() && !opt.disabled) e.currentTarget.style.background = 'transparent'
                  }}
                >
                  <div style={{ display: 'flex', 'align-items': 'center', gap: '0.5rem', flex: 1, overflow: 'hidden' }}>
                    <Show when={opt.icon}>{opt.icon!({ class: 'dropdown-opt-icon' })}</Show>
                    <span style={{ overflow: 'hidden', 'text-overflow': 'ellipsis', 'white-space': 'nowrap' }}>
                      {opt.label}
                    </span>
                    <Show when={isSelected()}>
                      <svg
                        width="14"
                        height="14"
                        viewBox="0 0 24 24"
                        fill="none"
                        stroke="currentColor"
                        stroke-width="2.5"
                        stroke-linecap="round"
                        stroke-linejoin="round"
                        style={{ 'flex-shrink': 0 }}
                      >
                        <polyline points="20 6 9 17 4 12" />
                      </svg>
                    </Show>
                  </div>
                  <Show when={opt.badge}>
                    <span
                      style={{
                        'font-size': '0.7rem',
                        padding: '0.12rem 0.45rem',
                        'border-radius': '4px',
                        'font-weight': 600,
                        'flex-shrink': 0,
                        'margin-left': '0.5rem',
                        ...getBadgeStyle(opt.badge!.variant),
                      }}
                    >
                      {opt.badge!.text}
                    </span>
                  </Show>
                </div>
              )
            }}
          </For>
        </div>
      </Show>
    </div>
  )
}
