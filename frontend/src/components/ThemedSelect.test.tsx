import { render, screen } from '@solidjs/testing-library'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { createSignal } from 'solid-js'
import { ThemedSelect, type ThemedSelectOption } from './ThemedSelect'

describe('ThemedSelect Component', () => {
  const options: ThemedSelectOption[] = [
    { value: 'opt1', label: 'Option One', badge: { text: 'Special', variant: 'emerald' } },
    { value: 'opt2', label: 'Option Two', badge: { text: 'Default', variant: 'slate' } },
    { value: 'opt3', label: 'Option Three', disabled: true },
  ]

  it('renders trigger with selected option label, badge, and closed chevron', () => {
    render(() => <ThemedSelect aria-label="Test Select" value="opt1" options={options} onChange={vi.fn()} />)

    const trigger = screen.getByRole('button', { name: /Option One/ })
    expect(trigger).toBeInTheDocument()
    expect(trigger).toHaveTextContent('Option One')
    expect(trigger).toHaveTextContent('Special')
    expect(screen.queryByRole('listbox')).not.toBeInTheDocument()
  })

  it('clicking trigger opens custom popover with role="listbox" and options', async () => {
    render(() => <ThemedSelect aria-label="Test Select" value="opt1" options={options} onChange={vi.fn()} />)

    const trigger = screen.getByRole('button', { name: /Option One/ })
    await userEvent.click(trigger)

    const listbox = screen.getByRole('listbox', { name: 'Test Select' })
    expect(listbox).toBeInTheDocument()

    const optElements = Array.from(listbox.querySelectorAll('[role="option"]'))
    expect(optElements.length).toBe(3)
    expect(optElements[0]).toHaveTextContent('Option One')
    expect(optElements[0]).toHaveAttribute('aria-selected', 'true')
    expect(optElements[1]).toHaveTextContent('Option Two')
    expect(optElements[1]).toHaveAttribute('aria-selected', 'false')
  })

  it('clicking an option calls onChange and closes popover', async () => {
    const onChange = vi.fn()
    render(() => <ThemedSelect aria-label="Test Select" value="opt1" options={options} onChange={onChange} />)

    const trigger = screen.getByRole('button', { name: /Option One/ })
    await userEvent.click(trigger)

    const listbox = screen.getByRole('listbox')
    const optElements = Array.from(listbox.querySelectorAll('[role="option"]'))
    await userEvent.click(optElements[1])

    expect(onChange).toHaveBeenCalledWith('opt2')
    expect(screen.queryByRole('listbox')).not.toBeInTheDocument()
  })

  it('clicking a disabled option does not call onChange and does not close popover', async () => {
    const onChange = vi.fn()
    render(() => <ThemedSelect aria-label="Test Select" value="opt1" options={options} onChange={onChange} />)

    const trigger = screen.getByRole('button', { name: /Option One/ })
    await userEvent.click(trigger)

    const listbox = screen.getByRole('listbox')
    const optElements = Array.from(listbox.querySelectorAll('[role="option"]'))
    await userEvent.click(optElements[2])

    expect(onChange).not.toHaveBeenCalled()
    expect(screen.getByRole('listbox')).toBeInTheDocument()
  })

  it('Escape key dismisses the popover', async () => {
    render(() => <ThemedSelect aria-label="Test Select" value="opt1" options={options} onChange={vi.fn()} />)

    await userEvent.click(screen.getByRole('button', { name: /Option One/ }))
    expect(screen.getByRole('listbox')).toBeInTheDocument()

    await userEvent.keyboard('{Escape}')
    expect(screen.queryByRole('listbox')).not.toBeInTheDocument()
  })

  it('clicking outside the container dismisses the popover', async () => {
    render(() => (
      <div>
        <button type="button">Outside Button</button>
        <ThemedSelect aria-label="Test Select" value="opt1" options={options} onChange={vi.fn()} />
      </div>
    ))

    await userEvent.click(screen.getByRole('button', { name: /Option One/ }))
    expect(screen.getByRole('listbox')).toBeInTheDocument()

    await userEvent.click(screen.getByRole('button', { name: 'Outside Button' }))
    expect(screen.queryByRole('listbox')).not.toBeInTheDocument()
  })

  it('supports testing compatibility via userEvent.selectOptions on hidden native select', async () => {
    function Parent() {
      const [val, setVal] = createSignal('opt1')
      return (
        <label>
          Test Label
          <ThemedSelect value={val()} options={options} onChange={setVal} />
        </label>
      )
    }
    render(() => <Parent />)

    // selectOptions on label-associated select element
    const select = screen.getByLabelText(/Test Label/)
    expect(select).toHaveValue('opt1')

    await userEvent.selectOptions(select, 'opt2')
    expect(select).toHaveValue('opt2')
    expect(screen.getByRole('button', { name: /Option Two/ })).toHaveTextContent('Option Two')
  })

  it('supports exact getByLabelText when aria-label is provided', async () => {
    function Parent() {
      const [val, setVal] = createSignal('opt1')
      return <ThemedSelect aria-label="Role" value={val()} options={options} onChange={setVal} />
    }
    render(() => <Parent />)

    const select = screen.getByLabelText('Role')
    expect(select).toHaveValue('opt1')

    await userEvent.selectOptions(select, 'opt2')
    expect(select).toHaveValue('opt2')
  })

  it('greys out placeholder text when no option is selected, and uses regular text color when selected', async () => {
    function Parent() {
      const [val, setVal] = createSignal('')
      return (
        <ThemedSelect
          aria-label="Account"
          value={val()}
          placeholder="Select an Account"
          options={options}
          onChange={setVal}
        />
      )
    }
    render(() => <Parent />)

    const trigger = screen.getByRole('button', { name: /Select an Account/ })
    const span = trigger.querySelector('span')
    expect(span).toHaveTextContent('Select an Account')
    expect(span).toHaveStyle({ color: 'var(--color-text-secondary, #9da3ae)' })

    // Open and select an option
    await userEvent.click(trigger)
    const listbox = screen.getByRole('listbox', { name: 'Account' })
    const optElements = Array.from(listbox.querySelectorAll('[role="option"]'))
    await userEvent.click(optElements[0])

    const updatedTrigger = screen.getByRole('button', { name: /Option One/ })
    const updatedSpan = updatedTrigger.querySelector('span')
    expect(updatedSpan).toHaveTextContent('Option One')
    expect(updatedSpan).toHaveStyle({ color: 'var(--color-text, #f0f0f2)' })
  })
})
