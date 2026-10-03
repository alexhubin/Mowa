export const route = $state({ pathname: window.location.pathname })

type Destination = {
  to: string
  params?: { inviteCode: string }
  replace?: boolean
}
export function navigate({ to, params, replace = false }: Destination) {
  const target = params
    ? to.replace('$inviteCode', encodeURIComponent(params.inviteCode))
    : to
  if (replace) window.history.replaceState(null, '', target)
  else window.history.pushState(null, '', target)
  route.pathname = window.location.pathname
}

export function syncLocation() {
  route.pathname = window.location.pathname
}

export function interceptLink(event: MouseEvent) {
  if (
    event.defaultPrevented ||
    event.button !== 0 ||
    event.metaKey ||
    event.ctrlKey ||
    event.shiftKey ||
    event.altKey
  )
    return
  const link =
    event.target instanceof Element ? event.target.closest('a') : null
  if (
    !link ||
    link.hasAttribute('download') ||
    (link.target && link.target !== '_self')
  )
    return
  // This URL is parsed once per click; it is not reactive application state.
  // eslint-disable-next-line svelte/prefer-svelte-reactivity
  const url = new URL(link.href)
  if (url.origin !== window.location.origin || url.hash) return
  event.preventDefault()
  navigate({ to: url.pathname + url.search })
}
