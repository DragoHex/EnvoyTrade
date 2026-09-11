import type { JSX } from 'solid-js'

export interface BrokerConfig {
  id: string
  name: string
  Logo: (props: { size?: number }) => JSX.Element
}

export function KiteLogo(props: { size?: number }) {
  const height = () => props.size ?? 18
  const width = () => Math.round(height() * 1.5)

  return (
    <svg
      viewBox="0 0 90 60"
      fill="none"
      xmlns="http://www.w3.org/2000/svg"
      class="broker-logo-svg"
      style={{ height: `${height()}px`, width: `${width()}px`, display: 'block' }}
      aria-hidden="true"
    >
      <polygon fill="#f6461a" points="30 0 0 30 30 60 60 30 90 0 30 0" />
      <polygon fill="#db342c" points="30 60 60 30 90 60 30 60" />
    </svg>
  )
}

function FallbackBrokerLogo(props: { name: string; size?: number }) {
  const height = () => props.size ?? 18
  return (
    <span
      class="broker-logo-fallback"
      style={{
        'font-size': `${Math.max(10, Math.round(height() * 0.65))}px`,
        'line-height': `${height()}px`,
      }}
    >
      {props.name}
    </span>
  )
}

const KITE_CONFIG: BrokerConfig = {
  id: 'kite',
  name: 'Zerodha Kite',
  Logo: KiteLogo,
}

export const SUPPORTED_BROKERS: BrokerConfig[] = [KITE_CONFIG]

export const BROKER_REGISTRY: Record<string, BrokerConfig> = {
  kite: KITE_CONFIG,
  zerodha: KITE_CONFIG,
}

export function getBrokerConfig(brokerId?: string): BrokerConfig {
  const key = (brokerId ?? '').toLowerCase().trim()
  if (key && BROKER_REGISTRY[key]) {
    return BROKER_REGISTRY[key]
  }

  const displayName = key ? key.charAt(0).toUpperCase() + key.slice(1) : 'Unknown Broker'
  return {
    id: key || 'unknown',
    name: displayName,
    Logo: (props) => <FallbackBrokerLogo name={displayName} size={props.size} />,
  }
}

export function BrokerLogo(props: {
  broker?: string
  size?: number
  showName?: boolean
  class?: string
}) {
  const config = () => getBrokerConfig(props.broker)

  return (
    <span
      class={`broker-logo-wrap ${props.class ?? ''}`.trim()}
      data-tooltip={config().name}
      title={config().name}
      aria-label={config().name}
    >
      {config().Logo({ size: props.size })}
      {props.showName && <span class="broker-logo-name">{config().name}</span>}
    </span>
  )
}
