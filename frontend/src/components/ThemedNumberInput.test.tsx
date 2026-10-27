import { render, fireEvent } from '@solidjs/testing-library'
import { createSignal } from 'solid-js'
import { describe, expect, it, vi } from 'vitest'
import { ThemedNumberInput } from './ThemedNumberInput'

describe('ThemedNumberInput', () => {
  it('renders input with value, id, and aria-label', () => {
    const { getByLabelText } = render(() => (
      <ThemedNumberInput
        id="test-input"
        aria-label="Test Counter"
        value={5}
        min={0}
        max={10}
      />
    ))

    const input = getByLabelText('Test Counter') as HTMLInputElement
    expect(input).toBeDefined()
    expect(input.id).toBe('test-input')
    expect(input.value).toBe('5')
    expect(input.min).toBe('0')
    expect(input.max).toBe('10')
  })

  it('handles typing via fireEvent.input', () => {
    const onInput = vi.fn()
    const { getByLabelText } = render(() => (
      <ThemedNumberInput
        aria-label="Count"
        value={2}
        onInput={onInput}
      />
    ))

    const input = getByLabelText('Count') as HTMLInputElement
    fireEvent.input(input, { target: { value: '7' } })
    expect(onInput).toHaveBeenCalled()
  })

  it('increments value on increment button click and fires input event', () => {
    const [val, setVal] = createSignal(4)
    const { getByLabelText, getByTitle } = render(() => (
      <ThemedNumberInput
        aria-label="Stepper Input"
        value={val()}
        min={0}
        max={10}
        onInput={(e) => setVal(parseInt(e.currentTarget.value) || 0)}
      />
    ))

    const upBtn = getByTitle('Increment')
    fireEvent.click(upBtn)
    expect(val()).toBe(5)

    const input = getByLabelText('Stepper Input') as HTMLInputElement
    expect(input.value).toBe('5')
  })

  it('decrements value on decrement button click and fires input event', () => {
    const [val, setVal] = createSignal(4)
    const { getByLabelText, getByTitle } = render(() => (
      <ThemedNumberInput
        aria-label="Stepper Input"
        value={val()}
        min={0}
        max={10}
        onInput={(e) => setVal(parseInt(e.currentTarget.value) || 0)}
      />
    ))

    const downBtn = getByTitle('Decrement')
    fireEvent.click(downBtn)
    expect(val()).toBe(3)

    const input = getByLabelText('Stepper Input') as HTMLInputElement
    expect(input.value).toBe('3')
  })

  it('respects min bound and disables decrement button', () => {
    const [val, setVal] = createSignal(-1)
    const { getByTitle } = render(() => (
      <ThemedNumberInput
        aria-label="Bounded Input"
        value={val()}
        min={-1}
        max={10}
        onInput={(e) => setVal(parseInt(e.currentTarget.value) || 0)}
      />
    ))

    const downBtn = getByTitle('Decrement') as HTMLButtonElement
    expect(downBtn.disabled).toBe(true)
    fireEvent.click(downBtn)
    expect(val()).toBe(-1)
  })

  it('respects max bound and disables increment button', () => {
    const [val, setVal] = createSignal(10)
    const { getByTitle } = render(() => (
      <ThemedNumberInput
        aria-label="Bounded Input"
        value={val()}
        min={0}
        max={10}
        onInput={(e) => setVal(parseInt(e.currentTarget.value) || 0)}
      />
    ))

    const upBtn = getByTitle('Increment') as HTMLButtonElement
    expect(upBtn.disabled).toBe(true)
    fireEvent.click(upBtn)
    expect(val()).toBe(10)
  })

  it('respects custom step attribute', () => {
    const [val, setVal] = createSignal(10)
    const { getByTitle } = render(() => (
      <ThemedNumberInput
        aria-label="Step Input"
        value={val()}
        step={5}
        min={0}
        max={100}
        onInput={(e) => setVal(parseInt(e.currentTarget.value) || 0)}
      />
    ))

    const upBtn = getByTitle('Increment')
    fireEvent.click(upBtn)
    expect(val()).toBe(15)

    const downBtn = getByTitle('Decrement')
    fireEvent.click(downBtn)
    expect(val()).toBe(10)
  })

  it('handles step="any" by incrementing by 0.1 without floating-point jitter', () => {
    const [val, setVal] = createSignal('1')
    const { getByTitle } = render(() => (
      <ThemedNumberInput
        aria-label="Clone Factor"
        value={val()}
        step="any"
        min={0.01}
        onInput={(e) => setVal(e.currentTarget.value)}
      />
    ))

    const upBtn = getByTitle('Increment')
    fireEvent.click(upBtn)
    expect(val()).toBe('1.1')

    fireEvent.click(upBtn)
    expect(val()).toBe('1.2')

    const downBtn = getByTitle('Decrement')
    fireEvent.click(downBtn)
    expect(val()).toBe('1.1')
  })


  it('disables stepper buttons when disabled prop is true', () => {
    const { getByTitle, getByLabelText } = render(() => (
      <ThemedNumberInput
        aria-label="Disabled Input"
        value={3}
        disabled={true}
      />
    ))

    const input = getByLabelText('Disabled Input') as HTMLInputElement
    const upBtn = getByTitle('Increment') as HTMLButtonElement
    const downBtn = getByTitle('Decrement') as HTMLButtonElement

    expect(input.disabled).toBe(true)
    expect(upBtn.disabled).toBe(true)
    expect(downBtn.disabled).toBe(true)
  })

  it('applies error class and styling when error prop is true', () => {
    const { container } = render(() => (
      <ThemedNumberInput
        aria-label="Error Input"
        value={0}
        error={true}
      />
    ))

    const input = container.querySelector('input')
    expect(input?.classList.contains('themed-number-input--error')).toBe(true)
  })
})
