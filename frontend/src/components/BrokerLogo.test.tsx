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

  it('ensures identical dimensions between Kite, Dhan, and Kotak Neo for standardized UI layout', () => {
    const { unmount: unmountKite } = render(() => <BrokerLogo broker="kite" size={20} />)
    const kiteSvg = screen.getByLabelText('Zerodha Kite').querySelector('svg')
    expect(kiteSvg).toHaveStyle({ height: '20px', width: '30px' })
    unmountKite()

    const { unmount: unmountDhan } = render(() => <BrokerLogo broker="dhan" size={20} />)
    const dhanSvg = screen.getByLabelText('Dhan').querySelector('svg')
    expect(dhanSvg).toHaveStyle({ height: '20px', width: '30px' })
    unmountDhan()

    render(() => <BrokerLogo broker="kotakneo" size={20} />)
    const kotakSvg = screen.getByLabelText('Kotak Neo').querySelector('svg')
    expect(kotakSvg).toHaveStyle({ height: '20px', width: '30px' })
  })

  it('renders official Kotak Neo logo for "kotakneo"', () => {
    render(() => <BrokerLogo broker="kotakneo" />)
    const logoEl = screen.getByLabelText('Kotak Neo')
    expect(logoEl).toBeInTheDocument()
    expect(logoEl).toHaveAttribute('data-tooltip', 'Kotak Neo')
    expect(logoEl).toHaveAttribute('title', 'Kotak Neo')
    const svg = logoEl.querySelector('svg.broker-logo-svg')
    expect(svg).not.toBeNull()
    expect(svg).toHaveAttribute('viewBox', '-15 0 135 90')
    const paths = svg?.querySelectorAll('path')
    expect(paths?.length).toBe(2)
    expect(paths?.[0]).toHaveAttribute('fill', '#ED1C24')
    expect(paths?.[1]).toHaveAttribute('fill', '#FFFFFF')
  })

  it('renders official Kotak Neo logo for alias "kotak"', () => {
    render(() => <BrokerLogo broker="kotak" />)
    const logoEl = screen.getByLabelText('Kotak Neo')
    expect(logoEl).toBeInTheDocument()
    expect(logoEl.querySelector('svg.broker-logo-svg')).not.toBeNull()
  })

  it('supports custom size for Kotak Neo logo with 1.5 aspect ratio', () => {
    render(() => <BrokerLogo broker="kotakneo" size={24} />)
    const logoEl = screen.getByLabelText('Kotak Neo')
    const svg = logoEl.querySelector('svg')
    expect(svg).toHaveStyle({ height: '24px', width: '36px' })
    expect(logoEl).toHaveStyle({ height: '24px', width: '36px' })
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

  it('renders testbroker with natural width and badge styling', () => {
    render(() => <BrokerLogo broker="testbroker" />)
    const logoEl = screen.getByLabelText('Test Broker (Mock)')
    expect(logoEl).toBeInTheDocument()
    expect(logoEl).toHaveStyle({ width: 'auto' })
    const badge = screen.getByText('TEST')
    expect(badge).toHaveClass('broker-logo-fallback')
    expect(badge).toHaveStyle({ 'text-overflow': 'ellipsis', overflow: 'hidden' })
  })

  it('renders fallback for unknown brokers with auto width without overflowing', () => {
    render(() => <BrokerLogo broker="custom_test_broker" />)
    const logoEl = screen.getByLabelText('Custom_test_broker')
    expect(logoEl).toHaveStyle({ width: 'auto' })
    const badge = screen.getByText('Custom_test_broker')
    expect(badge).toHaveStyle({ 'text-overflow': 'ellipsis', overflow: 'hidden' })
  })

  it('getBrokerConfig returns registered and fallback configs', () => {
    expect(getBrokerConfig('kite').name).toBe('Zerodha Kite')
    expect(getBrokerConfig('KITE').name).toBe('Zerodha Kite')
    expect(getBrokerConfig('zerodha').name).toBe('Zerodha Kite')
    expect(getBrokerConfig('dhan').name).toBe('Dhan')
    expect(getBrokerConfig('DHAN').name).toBe('Dhan')
    expect(getBrokerConfig('kotakneo').name).toBe('Kotak Neo')
    expect(getBrokerConfig('KOTAKNEO').name).toBe('Kotak Neo')
    expect(getBrokerConfig('kotak').name).toBe('Kotak Neo')
    expect(getBrokerConfig('other').name).toBe('Other')
    expect(getBrokerConfig('').name).toBe('Unknown Broker')
  })
})
