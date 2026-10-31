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

export function DhanLogo(props: { size?: number }) {
  const height = () => props.size ?? 18
  const width = () => Math.round(height() * 1.5)

  return (
    <svg
      viewBox="-11.25 0 67.5 45"
      fill="none"
      xmlns="http://www.w3.org/2000/svg"
      class="broker-logo-svg"
      style={{ height: `${height()}px`, width: `${width()}px`, display: 'block' }}
      aria-hidden="true"
    >
      <path d="M22.5 45C34.9263 45 45 34.9264 45 22.5C45 10.0736 34.9263 0 22.5 0C10.0736 0 0 10.0736 0 22.5C0 34.9264 10.0736 45 22.5 45Z" fill="url(#paint0_linear_dhan)"/>
      <path fill-rule="evenodd" clip-rule="evenodd" d="M32.7051 42.5445L14.9963 28.8399L17.4969 22.4963L13.2656 16.7293L14.9963 11.5391H32.881L44.9964 22.5524C44.9751 31.2764 39.9848 38.8331 32.7051 42.5445Z" fill="#17875F"/>
      <path d="M34.6073 14.6895H29.4749V34.9523C28.6379 34.9523 27.8135 34.7924 27.0012 34.4725C26.1888 34.1772 25.7826 33.8328 25.7826 33.439V29.047C24.0596 30.1297 22.1888 30.6709 20.1703 30.6709C18.1518 30.6709 16.5027 30.1542 15.2227 29.1207C13.9674 28.0627 13.3396 26.6971 13.3396 25.0239C13.3396 23.7445 13.7827 22.6003 14.6688 21.5914C13.4135 21.1239 12.4165 20.4349 11.678 19.5244C10.9396 18.5895 10.5703 17.4821 10.5703 16.2028C10.5703 14.9233 11.0873 13.7545 12.1211 12.6964C13.1796 11.6384 14.6196 11.1094 16.4412 11.1094C17.7703 11.1094 18.7918 11.4046 19.5058 11.9952C20.2442 12.5857 20.6135 13.3115 20.6135 14.1728C20.6135 15.9198 19.481 17.15 17.2166 17.8636C16.8474 17.5684 16.5765 17.15 16.4042 16.6088C17.4133 15.9444 17.9181 15.2432 17.9181 14.5049C17.9181 14.0866 17.7949 13.7545 17.5488 13.5084C17.3026 13.2377 16.9088 13.1024 16.3673 13.1024C15.8257 13.1024 15.2965 13.3978 14.7796 13.9883C14.2627 14.5788 14.0042 15.3538 14.0042 16.3134C14.0042 17.2485 14.2627 18.0358 14.7796 18.6757C15.3211 19.2907 15.998 19.7214 16.8103 19.9674C17.8933 19.4262 19.1118 19.0202 20.4658 18.7494C21.4011 19.6353 21.9918 20.5579 22.2381 21.5175C20.7118 21.8128 19.4196 22.2803 18.3612 22.9202C17.3273 23.5353 16.8103 24.2489 16.8103 25.0608C16.8103 25.8482 17.0811 26.4879 17.6227 26.9801C18.1888 27.4722 18.9888 27.7182 20.0226 27.7182C21.081 27.7182 22.1273 27.423 23.1612 26.8324C24.195 26.2418 25.0688 25.4668 25.7826 24.5071V14.6895H23.2349C22.3488 13.656 21.9058 12.5611 21.9058 11.4046H33.4627C34.2257 12.3888 34.6073 13.4838 34.6073 14.6895Z" fill="white"/>
      <defs>
        <linearGradient id="paint0_linear_dhan" x1="42.1875" y1="38.25" x2="26.1224" y2="-0.611659" gradientUnits="userSpaceOnUse">
          <stop stop-color="#138C61"/>
          <stop offset="1" stop-color="#4DBA93" stop-opacity="0.99"/>
        </linearGradient>
      </defs>
    </svg>
  )
}

function FallbackBrokerLogo(props: { name: string; size?: number }) {
  const height = () => props.size ?? 18
  const width = () => Math.round(height() * 1.5)
  return (
    <span
      class="broker-logo-fallback"
      style={{
        width: 'auto',
        'min-width': `${width()}px`,
        'max-width': '100%',
        height: `${height()}px`,
        'font-size': `${Math.max(10, Math.round(height() * 0.65))}px`,
        'line-height': `${height()}px`,
        'text-align': 'center',
        display: 'inline-flex',
        'align-items': 'center',
        'justify-content': 'center',
        'box-sizing': 'border-box',
        padding: '0 5px',
        overflow: 'hidden',
        'text-overflow': 'ellipsis',
      }}
    >
      {props.name}
    </span>
  )
}

export function KotakLogo(props: { size?: number }) {
  const height = () => props.size ?? 18
  const width = () => Math.round(height() * 1.5)

  return (
    <svg
      viewBox="-15 0 135 90"
      fill="none"
      xmlns="http://www.w3.org/2000/svg"
      class="broker-logo-svg"
      style={{ height: `${height()}px`, width: `${width()}px`, display: 'block' }}
      aria-hidden="true"
    >
      <path
        d="M0 45.096C0 20.192 23.474 0 52.434 0s52.432 20.192 52.432 45.096-23.474 45.096-52.432 45.096S0 70.003 0 45.096"
        fill="#ED1C24"
      />
      <path
        d="M91.866 49.636c-1.143 9.42-6.427 18.937-17.965 18.955-6.736.012-11.998-4.6-15.642-10.243v-6.536c4.56 2.264 8.822 4.694 13.993 4.768 6.37.094 12.13-2.355 14.512-6.945h5.103zm-38.57 5.915c-5.293 6.38-10.408 12.897-19.992 12.897-13.537 0-19.958-13.088-19.958-24.03 0-10.514 5.018-22.96 18.206-22.96 5.72 0 11.226 3.498 14.508 7.198l-.01 8.925c-2.742-1.846-8.884-3.657-12.894-3.722-8.363-.14-15.736 3.485-15.608 11.74.087 5.683 5.718 9.546 11.32 9.546 8.58 0 13.778-7.838 18.122-13.787a490.38 490.38 0 0 0 5.165-6.858c4.825-6.742 10.408-12.897 19.992-12.897 11.29 0 17.628 9.1 19.42 18.468h-5.1c-2.05-3.038-5.993-4.897-9.877-4.897-8.87 0-14.17 8.16-18.606 14.168l-4.7 6.208"
        fill="#FFFFFF"
      />
    </svg>
  )
}

const KITE_CONFIG: BrokerConfig = {
  id: 'kite',
  name: 'Zerodha Kite',
  Logo: KiteLogo,
}

const DHAN_CONFIG: BrokerConfig = {
  id: 'dhan',
  name: 'Dhan',
  Logo: DhanLogo,
}

const KOTAK_CONFIG: BrokerConfig = {
  id: 'kotakneo',
  name: 'Kotak Neo',
  Logo: KotakLogo,
}

const TESTBROKER_CONFIG: BrokerConfig = {
  id: 'testbroker',
  name: 'Test Broker (Mock)',
  Logo: (props) => <FallbackBrokerLogo name="TEST" size={props.size} />,
}

export const SUPPORTED_BROKERS: BrokerConfig[] = [KITE_CONFIG, DHAN_CONFIG, KOTAK_CONFIG]

export const BROKER_REGISTRY: Record<string, BrokerConfig> = {
  kite: KITE_CONFIG,
  zerodha: KITE_CONFIG,
  dhan: DHAN_CONFIG,
  kotakneo: KOTAK_CONFIG,
  kotak: KOTAK_CONFIG,
  testbroker: TESTBROKER_CONFIG,
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
  const isSvg = () => {
    const k = (props.broker ?? '').toLowerCase().trim()
    return k === 'kite' || k === 'zerodha' || k === 'dhan' || k === 'kotakneo' || k === 'kotak'
  }
  const height = () => props.size ?? 18
  const width = () => Math.round(height() * 1.5)

  return (
    <span
      class={`broker-logo-wrap ${props.class ?? ''}`.trim()}
      data-tooltip={config().name}
      title={config().name}
      aria-label={config().name}
      style={{
        width: props.showName || !isSvg() ? 'auto' : `${width()}px`,
        height: `${height()}px`,
        'min-width': `${width()}px`,
        'max-width': '100%',
        'flex-shrink': '0',
        display: 'inline-flex',
        'align-items': 'center',
        'justify-content': 'center',
      }}
    >
      {config().Logo({ size: props.size })}
      {props.showName && <span class="broker-logo-name">{config().name}</span>}
    </span>
  )
}
