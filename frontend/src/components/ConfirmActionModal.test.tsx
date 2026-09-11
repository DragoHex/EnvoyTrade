import { render, screen } from '@solidjs/testing-library'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { ConfirmActionModal } from './ConfirmActionModal'

describe('ConfirmActionModal', () => {
  it('renders nothing when closed', () => {
    render(() => <ConfirmActionModal open={false} label="Square Off" onConfirm={() => {}} onCancel={() => {}} />)
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  })

  it('calls onConfirm when Confirm is clicked', async () => {
    const onConfirm = vi.fn()
    render(() => <ConfirmActionModal open={true} label="Square Off" onConfirm={onConfirm} onCancel={() => {}} />)
    await userEvent.click(screen.getByText('Confirm'))
    expect(onConfirm).toHaveBeenCalled()
  })

  it('calls onCancel when Cancel is clicked', async () => {
    const onCancel = vi.fn()
    render(() => <ConfirmActionModal open={true} label="Square Off" onConfirm={() => {}} onCancel={onCancel} />)
    await userEvent.click(screen.getByText('Cancel'))
    expect(onCancel).toHaveBeenCalled()
  })
})
