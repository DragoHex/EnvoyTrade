import type { JSX } from 'solid-js'

export interface EnvoyLogoProps {
  size?: number
  class?: string
  style?: JSX.CSSProperties | string
  accentColor?: string
  secondaryColor?: string
  ariaLabel?: string
  'data-testid'?: string
}

/**
 * EnvoyTrade Brand Logo.
 * Designed with Golden Ratio (phi ≈ 1.618) and Fibonacci circle proportions (3, 5, 8, 13, 21):
 * - Outer trajectory radius: 21px
 * - Inner harmonic wave radius: 13px (21/13 ≈ 1.618)
 * - Arrowhead span: 8px (13/8 ≈ 1.625)
 * - Inter-trajectory gap: 5px (8/5 = 1.600)
 * - Stroke weight: 3.5px
 *
 * Symbolism:
 * - Upper green trajectory: The Master trader forging the market path ahead.
 * - Lower secondary trajectory: The Follower executing with zero latency in lockstep.
 * - Overall silhouette: Monogram 'E' and dual bullish vectors (↗).
 */
export function EnvoyLogo(props: EnvoyLogoProps) {
  const size = () => props.size ?? 24
  const accent = () => props.accentColor ?? 'var(--color-accent, #06d896)'
  const secondary = () => props.secondaryColor ?? 'currentColor'

  return (
    <svg
      viewBox="0 0 48 48"
      fill="none"
      xmlns="http://www.w3.org/2000/svg"
      class={props.class}
      style={{
        width: `${size()}px`,
        height: `${size()}px`,
        display: 'inline-block',
        'vertical-align': 'middle',
        'flex-shrink': '0',
        ...(typeof props.style === 'object' ? props.style : {}),
      }}
      role="img"
      aria-label={props.ariaLabel ?? 'EnvoyTrade Logo'}
      data-testid={props['data-testid'] ?? 'envoy-logo'}
    >
      {/* Master Trajectory (Emerald Accent) */}
      <path
        d="M 8 21.5 C 8 12.5, 15 5.5, 25 5.5 C 31.5 5.5, 36.5 8.5, 39.5 12"
        stroke={accent()}
        stroke-width="3.5"
        stroke-linecap="round"
      />
      <path
        d="M 8 21.5 C 13.5 21.5, 18.5 18, 25.5 15.5 C 30.5 13.5, 34.5 12, 38 12"
        stroke={accent()}
        stroke-width="3.5"
        stroke-linecap="round"
      />
      <path d="M 33 6 L 42.5 11 L 37.5 19 Z" fill={accent()} />

      {/* Follower Trajectory (Secondary Slate / Platinum) */}
      <path
        d="M 9.5 26.5 C 9.5 35.5, 16.5 42.5, 26.5 42.5 C 34 42.5, 39.5 37.5, 41.5 30.5"
        stroke={secondary()}
        stroke-width="3.5"
        stroke-linecap="round"
      />
      <path
        d="M 9.5 26.5 C 15 26.5, 20.5 24.5, 26.5 22.5 C 31.5 21, 35 20, 38 20"
        stroke={secondary()}
        stroke-width="3.5"
        stroke-linecap="round"
      />
      <path d="M 33.5 15 L 43 20 L 38 28 Z" fill={secondary()} />
    </svg>
  )
}
