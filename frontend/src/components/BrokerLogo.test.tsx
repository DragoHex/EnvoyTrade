import { render, screen } from '@solidjs/testing-library'
import { describe, expect, it } from 'vitest'
import { BrokerLogo, getBrokerConfig } from './BrokerLogo'

describe('BrokerLogo', () => {
  it('renders official Kite logo for "kite"', () => {
    render(() => <BrokerLogo broker="kite" />)
    const logoEl = screen.getByLabelText('Zerodha Kite')
    expect(logoEl).toBeInTheDocument()
    expect(logoEl).toHaveAttribute('data-tooltip', 'Zerodha Kite')
    expect(logoEl).toHaveAttribute('title', 'Zerodha Kite')
    expect(logoEl.querySelector('svg.broker-logo-svg')).not.toBeNull()
  })

  it('renders official Kite logo for alias "zerodha"', () => {
    render(() => <BrokerLogo broker="zerodha" />)
    const logoEl = screen.getByLabelText('Zerodha Kite')
    expect(logoEl).toBeInTheDocument()
    expect(logoEl.querySelector('svg.broker-logo-svg')).not.toBeNull()
  })

  it('renders fallback badge for unknown brokers without crashing', () => {
    render(() => <BrokerLogo broker="custom_broker" />)
    const logoEl = screen.getByLabelText('Custom_broker')
    expect(logoEl).toBeInTheDocument()
    expect(screen.getByText('Custom_broker')).toBeInTheDocument()
  })

  it('handles empty or undefined broker gracefully', () => {
    render(() => <BrokerLogo />)
    const logoEl = screen.getByLabelText('Unknown Broker')
    expect(logoEl).toBeInTheDocument()
    expect(screen.getByText('Unknown Broker')).toBeInTheDocument()
  })

  it('supports custom size and showName', () => {
    render(() => <BrokerLogo broker="kite" size={24} showName />)
    const logoEl = screen.getByLabelText('Zerodha Kite')
    const svg = logoEl.querySelector('svg')
    expect(svg).toHaveStyle({ height: '24px' })
    expect(screen.getByText('Zerodha Kite')).toBeInTheDocument()
  })

  it('getBrokerConfig returns registered and fallback configs', () => {
    expect(getBrokerConfig('kite').name).toBe('Zerodha Kite')
    expect(getBrokerConfig('KITE').name).toBe('Zerodha Kite')
    expect(getBrokerConfig('zerodha').name).toBe('Zerodha Kite')
    expect(getBrokerConfig('other').name).toBe('Other')
    expect(getBrokerConfig('').name).toBe('Unknown Broker')
  })
})
