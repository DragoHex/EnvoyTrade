import { render, screen } from '@solidjs/testing-library'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { ThemeToggle } from './ThemeToggle'

describe('ThemeToggle', () => {
  it('reflects the dark prop via aria-checked', () => {
    render(() => <ThemeToggle dark={true} onToggle={() => {}} />)
    expect(screen.getByRole('switch')).toHaveAttribute('aria-checked', 'true')
  })

  it('calls onToggle when clicked', async () => {
    const onToggle = vi.fn()
    render(() => <ThemeToggle dark={false} onToggle={onToggle} />)
    await userEvent.click(screen.getByRole('switch'))
    expect(onToggle).toHaveBeenCalled()
  })
})
