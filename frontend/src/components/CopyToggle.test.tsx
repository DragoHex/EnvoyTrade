import { render, screen } from '@solidjs/testing-library'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { CopyToggle } from './CopyToggle'

describe('CopyToggle', () => {
  it('reflects the enabled prop', () => {
    render(() => <CopyToggle enabled={true} onToggle={() => {}} />)
    expect(screen.getByRole('checkbox')).toBeChecked()
  })

  it('calls onToggle with the new value when clicked', async () => {
    const onToggle = vi.fn()
    render(() => <CopyToggle enabled={true} onToggle={onToggle} />)
    await userEvent.click(screen.getByRole('checkbox'))
    expect(onToggle).toHaveBeenCalledWith(false)
  })

  it('disables the checkbox when disabled is true', () => {
    render(() => <CopyToggle enabled={true} disabled onToggle={() => {}} />)
    expect(screen.getByRole('checkbox')).toBeDisabled()
  })
})
