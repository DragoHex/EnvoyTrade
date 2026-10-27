import { type JSX, createMemo, splitProps } from 'solid-js'

export interface ThemedNumberInputProps {
  id?: string
  name?: string
  value?: number | string
  min?: number | string
  max?: number | string
  step?: number | string
  placeholder?: string
  disabled?: boolean
  readonly?: boolean
  required?: boolean
  error?: boolean
  class?: string
  style?: JSX.CSSProperties | string
  containerStyle?: JSX.CSSProperties | string
  'aria-label'?: string
  ariaLabel?: string
  onInput?: JSX.EventHandlerUnion<HTMLInputElement, InputEvent>
  onChange?: JSX.EventHandlerUnion<HTMLInputElement, Event>
  onBlur?: JSX.EventHandlerUnion<HTMLInputElement, FocusEvent>
}

export function ThemedNumberInput(props: ThemedNumberInputProps) {
  let inputRef: HTMLInputElement | undefined

  const [local, inputProps] = splitProps(props, [
    'value',
    'min',
    'max',
    'step',
    'disabled',
    'readonly',
    'error',
    'class',
    'style',
    'containerStyle',
    'aria-label',
    'ariaLabel',
    'onInput',
    'onChange',
  ])

  const numVal = createMemo(() => {
    if (local.value === undefined || local.value === '') return NaN
    const n = typeof local.value === 'number' ? local.value : parseFloat(String(local.value))
    return isNaN(n) ? NaN : n
  })

  const minVal = createMemo(() => (local.min !== undefined ? parseFloat(String(local.min)) : -Infinity))
  const maxVal = createMemo(() => (local.max !== undefined ? parseFloat(String(local.max)) : Infinity))
  const stepVal = createMemo(() => {
    if (local.step === undefined) return 1
    if (local.step === 'any') return 0.1
    const s = parseFloat(String(local.step))
    return isNaN(s) ? 1 : s
  })

  const isAtMin = createMemo(() => !isNaN(numVal()) && numVal() <= minVal())
  const isAtMax = createMemo(() => !isNaN(numVal()) && numVal() >= maxVal())

  const handleStep = (direction: 1 | -1) => {
    if (local.disabled || local.readonly || !inputRef) return
    if (direction === 1 && isAtMax()) return
    if (direction === -1 && isAtMin()) return

    const step = stepVal()
    let next: number

    if (isNaN(numVal())) {
      if (local.min !== undefined && minVal() > 0) {
        next = direction === 1 ? minVal() : minVal()
      } else {
        next = direction === 1 ? step : -step
      }
    } else {
      const current = numVal()
      const rawNext = direction === 1 
        ? Math.min(maxVal(), current + step)
        : Math.max(minVal(), current - step)

      const stepStr = String(step)
      const dotIdx = stepStr.indexOf('.')
      const decimals = dotIdx >= 0 ? stepStr.length - dotIdx - 1 : 0
      const precision = Math.max(decimals, 0)
      next = parseFloat(rawNext.toFixed(precision))
    }

    inputRef.value = String(next)
    inputRef.dispatchEvent(new Event('input', { bubbles: true }))
    inputRef.dispatchEvent(new Event('change', { bubbles: true }))
  }

  return (
    <div
      class="themed-number-wrapper"
      style={local.containerStyle}
    >
      <input
        ref={inputRef}
        {...inputProps}
        type="number"
        value={local.value !== undefined ? local.value : ''}
        min={local.min}
        max={local.max}
        step={local.step}
        disabled={local.disabled}
        readonly={local.readonly}
        aria-label={local['aria-label'] || local.ariaLabel}
        class={`themed-number-input ${local.error ? 'themed-number-input--error' : ''} ${local.class || ''}`}
        style={local.style}
        onInput={(e) => {
          if (typeof local.onInput === 'function') {
            local.onInput(e)
          }
        }}
        onChange={(e) => {
          if (typeof local.onChange === 'function') {
            local.onChange(e)
          }
        }}
      />
      <div class="themed-number-steppers">
        <button
          type="button"
          tabIndex={-1}
          class="themed-number-btn themed-number-btn-up"
          title="Increment"
          aria-label="Increment"
          disabled={local.disabled || local.readonly || isAtMax()}
          onClick={() => handleStep(1)}
        >
          <svg viewBox="0 0 16 16" fill="currentColor" width="10" height="10" aria-hidden="true">
            <path d="M7.293 4.707a1 1 0 0 1 1.414 0l4 4a1 1 0 0 1-1.414 1.414L8 6.828 4.707 10.12a1 1 0 0 1-1.414-1.414l4-4z" />
          </svg>
        </button>
        <button
          type="button"
          tabIndex={-1}
          class="themed-number-btn themed-number-btn-down"
          title="Decrement"
          aria-label="Decrement"
          disabled={local.disabled || local.readonly || isAtMin()}
          onClick={() => handleStep(-1)}
        >
          <svg viewBox="0 0 16 16" fill="currentColor" width="10" height="10" aria-hidden="true">
            <path d="M8.707 11.293a1 1 0 0 1-1.414 0l-4-4a1 1 0 1 1 1.414-1.414L8 9.172l3.293-3.293a1 1 0 1 1 1.414 1.414l-4 4z" />
          </svg>
        </button>
      </div>
    </div>
  )
}
