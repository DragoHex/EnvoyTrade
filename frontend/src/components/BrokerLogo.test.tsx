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

  it('renders official Dhan logo for "dhan"', () => {
    render(() => <BrokerLogo broker="dhan" />)
    const logoEl = screen.getByLabelText('Dhan')
    expect(logoEl).toBeInTheDocument()
    expect(logoEl).toHaveAttribute('data-tooltip', 'Dhan')
    expect(logoEl).toHaveAttribute('title', 'Dhan')
    expect(logoEl.querySelector('svg.broker-logo-svg')).not.toBeNull()
  })

  it('supports custom size for Dhan logo with 1.5 aspect ratio', () => {
    render(() => <BrokerLogo broker="dhan" size={24} />)
    const logoEl = screen.getByLabelText('Dhan')
    const svg = logoEl.querySelector('svg')
    expect(svg).toHaveStyle({ height: '24px', width: '36px' })
    expect(logoEl).toHaveStyle({ height: '24px', width: '36px' })
  })

  it('ensures identical dimensions between Kite and Dhan for standardized UI layout', () => {
    const { unmount: unmountKite } = render(() => <BrokerLogo broker="kite" size={20} />)
    const kiteSvg = screen.getByLabelText('Zerodha Kite').querySelector('svg')
    expect(kiteSvg).toHaveStyle({ height: '20px', width: '30px' })
    unmountKite()

    render(() => <BrokerLogo broker="dhan" size={20} />)
    const dhanSvg = screen.getByLabelText('Dhan').querySelector('svg')
    expect(dhanSvg).toHaveStyle({ height: '20px', width: '30px' })
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
    expect(getBrokerConfig('dhan').name).toBe('Dhan')
    expect(getBrokerConfig('DHAN').name).toBe('Dhan')
    expect(getBrokerConfig('other').name).toBe('Other')
    expect(getBrokerConfig('').name).toBe('Unknown Broker')
  })
})
