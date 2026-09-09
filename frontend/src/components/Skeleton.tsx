import { For, Show, createSignal, onCleanup } from 'solid-js'

const LOADING_TIMEOUT_MS = 60_000

// Wrap a Suspense fallback in this so a stuck request doesn't skeleton
// forever — after a minute we assume something's wrong and say so instead.
export function LoadingTimeout(props: { children: any }) {
  const [timedOut, setTimedOut] = createSignal(false)
  const timer = setTimeout(() => setTimedOut(true), LOADING_TIMEOUT_MS)
  onCleanup(() => clearTimeout(timer))

  return (
    <Show when={!timedOut()} fallback={<p>Still loading — this is taking longer than expected.</p>}>
      {props.children}
    </Show>
  )
}

export function GroupCardSkeleton() {
  return (
    <section class="skeleton-card" aria-hidden="true">
      <div class="skeleton-bar skeleton-title" />
      <div class="skeleton-table">
        <For each={[0, 1, 2, 3, 4]}>{() => <div class="skeleton-bar skeleton-row" />}</For>
      </div>
    </section>
  )
}

export function GroupsSkeleton() {
  return (
    <>
      <GroupCardSkeleton />
      <GroupCardSkeleton />
    </>
  )
}
