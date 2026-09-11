import { Show } from 'solid-js'
import { GandalfIcon, HobbitIcon } from './icons'

export function RoleIcon(props: {
  role: 'master' | 'follower'
  size?: number
  class?: string
}) {
  const label = () => (props.role === 'master' ? 'Master' : 'Follower')

  return (
    <span
      class={`role-icon-wrap ${props.class ?? ''}`.trim()}
      data-tooltip={label()}
      title={label()}
      aria-label={label()}
      role="img"
    >
      <Show when={props.role === 'master'} fallback={<HobbitIcon size={props.size} />}>
        <GandalfIcon size={props.size} />
      </Show>
    </span>
  )
}
