import { render, screen } from '@solidjs/testing-library'
import { describe, expect, it, beforeEach } from 'vitest'
import { Router, Route } from '@solidjs/router'
import { NotFoundPage } from './NotFoundPage'

describe('NotFoundPage', () => {
  beforeEach(() => {
    window.history.pushState({}, '', '/404')
  })

  it('renders 404 code, not found message, themed cartoon, and dashboard link', () => {
    render(() => (
      <Router>
        <Route path="/404" component={NotFoundPage} />
      </Router>
    ))

    expect(screen.getByRole('heading', { level: 1 })).toHaveTextContent('404')
    expect(screen.getByRole('heading', { level: 2 })).toHaveTextContent('Page Not Found')
    expect(screen.getByText(/I have no memory of this place/i)).toBeInTheDocument()

    const img = screen.getByTestId('not-found-image')
    expect(img).toBeInTheDocument()
    expect(img).toHaveAttribute('alt', expect.stringContaining('Gandalf'))

    const returnBtn = screen.getByTestId('btn-return-dashboard')
    expect(returnBtn).toBeInTheDocument()
    expect(returnBtn).toHaveAttribute('href', '/')
    expect(returnBtn).toHaveTextContent('Return to Dashboard')
  })
})
