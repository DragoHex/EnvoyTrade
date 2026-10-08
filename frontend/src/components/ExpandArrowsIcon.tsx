import expandArrowsUrl from '../assets/expand-arrows.png'

export function ExpandArrowsIcon(props: { class?: string; size?: number }) {
  const size = () => props.size ?? 14
  return (
    <span
      class={`expand-arrows-icon ${props.class ?? ''}`.trim()}
      style={{
        display: 'inline-block',
        width: `${size()}px`,
        height: `${size()}px`,
        'background-color': 'currentColor',
        '-webkit-mask': `url(${expandArrowsUrl}) no-repeat center / contain`,
        mask: `url(${expandArrowsUrl}) no-repeat center / contain`,
        'vertical-align': 'middle',
      }}
      role="img"
      aria-hidden="true"
    />
  )
}
