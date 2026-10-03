<script lang="ts">
  import {
    createQuery,
    createMutation,
    useQueryClient,
  } from '@tanstack/svelte-query'
  import { navigate, route } from '../navigation.svelte'

  import { Phone, PhoneOff } from '@lucide/svelte'
  import { api, currentUser, type DirectCall } from '../api'
  import { initials } from '../utils'
  const queryClient = useQueryClient()
  const pathname = $derived(route.pathname)
  const userQuery = createQuery(() => ({
    queryKey: ['me'],
    queryFn: currentUser,
  }))
  const user = $derived(userQuery.data)
  const calls = createQuery(() => ({
    queryKey: ['calls'],
    queryFn: () => api<DirectCall[]>('/api/calls'),
    enabled: Boolean(user && !user.must_change_password),
    refetchInterval: user ? 30_000 : false,
  }))
  $effect(() => {
    if (!user || user.must_change_password) return
    const events = new EventSource('/api/calls/events')
    const refreshCalls = () =>
      void queryClient.invalidateQueries({ queryKey: ['calls'] })
    events.addEventListener('calls', refreshCalls)
    return () => {
      events.removeEventListener('calls', refreshCalls)
      events.close()
    }
  })
  $effect(() => {
    if (!user || user.must_change_password) return
    const heartbeat = () =>
      void api<void>('/api/presence', { method: 'POST' }).catch(() => undefined)
    heartbeat()
    const interval = window.setInterval(heartbeat, 10_000)
    return () => window.clearInterval(interval)
  })
  const incoming = $derived(
    calls.data?.find((call) => call.incoming && call.status === 'ringing'),
  )
  const accept = createMutation(() => ({
    mutationFn: () =>
      api<DirectCall>(`/api/calls/${incoming!.id}/accept`, { method: 'POST' }),
    onSuccess: async (call) => {
      await queryClient.invalidateQueries({ queryKey: ['calls'] })
      await navigate({
        to: '/r/$inviteCode',
        params: { inviteCode: call.invite_code },
      })
    },
  }))
  const decline = createMutation(() => ({
    mutationFn: () =>
      api<void>(`/api/calls/${incoming!.id}/decline`, { method: 'POST' }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['calls'] }),
  }))
</script>

{#if incoming && !pathname.startsWith('/r/')}
  <div
    class="incoming-call"
    role="dialog"
    aria-label={`Incoming call from ${incoming.peer.display_name}`}
  >
    <div class="participant-avatar">{initials(incoming.peer.display_name)}</div>
    <div class="min-w-0 flex-1">
      <p
        class="text-xs font-semibold uppercase tracking-[.12em] text-ink-muted"
      >
        Incoming call
      </p>
      <strong class="mt-1 block truncate">{incoming.peer.display_name}</strong
      ><span class="text-xs text-ink-muted">{incoming.peer.username}</span>
    </div>
    <button
      class="call-answer"
      onclick={() => accept.mutate()}
      disabled={accept.isPending}
      aria-label="Accept"><Phone size={19} /></button
    >
    <button
      class="call-decline"
      onclick={() => decline.mutate()}
      disabled={decline.isPending}
      aria-label="Decline"><PhoneOff size={19} /></button
    >
  </div>
{/if}
