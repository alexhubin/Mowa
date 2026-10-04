<script lang="ts">
  import { onMount } from 'svelte'
  import { createMutation, createQuery, useQueryClient } from '@tanstack/svelte-query'
  import { navigate } from '../navigation.svelte'
  import { api, type User } from '../api'

  const queryClient = useQueryClient()
  const params = new URLSearchParams(window.location.search)
  const next = params.get('next') ?? '/'
  let stage = $state<'email' | 'code' | 'username'>('email')
  let email = $state('')
  let code = $state('')
  let username = $state('')
  let retryIn = $state(0)
  let error = $state(params.has('auth_error') ? 'Google sign-in failed. Please try again or use an email code.' : '')
  type Result = { user?: User; next?: string; needs_username?: boolean; email?: string }
  const methods = createQuery(() => ({ queryKey: ['auth-methods'], queryFn: () => api<{ email: boolean; google: boolean }>('/api/auth/methods') }))
  async function finish(result: Result) {
    error = ''
    if (result.needs_username) { email = result.email ?? email; stage = 'username'; return }
    if (!result.user) throw new Error('Could not sign in. Please try again.')
    queryClient.setQueryData(['me'], result.user)
    const destination = result.next ?? next
    if (destination.startsWith('/') && !destination.startsWith('//') && !destination.includes('\\')) {
      await navigate({ to: destination })
    } else await navigate({ to: '/' })
  }
  const send = createMutation(() => ({
    mutationFn: () => api<{ retry_after: number }>('/api/auth/email/start', { method: 'POST', body: JSON.stringify({ email, next }) }),
    onSuccess: (result) => { stage = 'code'; code = ''; retryIn = result.retry_after; error = '' },
    onError: (e) => { error = e.message },
  }))
  const verify = createMutation(() => ({
    mutationFn: () => api<Result>(stage === 'username' ? '/api/auth/complete' : '/api/auth/email/verify', { method: 'POST', body: JSON.stringify(stage === 'username' ? { username } : { code }) }),
    onSuccess: finish,
    onError: (e) => { error = e.message },
  }))
  const resume = createMutation(() => ({
    mutationFn: () => api<Result>('/api/auth/identity', { method: 'POST' }),
    onSuccess: finish,
    onError: (e) => { error = e.message },
  }))
  const busy = $derived(send.isPending || verify.isPending || resume.isPending)
  onMount(() => {
    if (params.has('verified')) resume.mutate()
    const timer = window.setInterval(() => { if (retryIn > 0) retryIn-- }, 1000)
    return () => window.clearInterval(timer)
  })
  function submit(event: SubmitEvent) {
    event.preventDefault()
    error = ''
    if (stage === 'email') send.mutate()
    else verify.mutate()
  }
</script>

<main class="auth-screen">
  <section class="auth-card">
    <div class="brand auth-brand"><span class="brand-dot" aria-hidden="true"></span><span>mowa</span></div>
    <h1 class="first-password-title">{stage === 'username' ? 'Choose your username' : stage === 'code' ? 'Check your email' : 'Sign in'}</h1>
    {#if stage === 'email' && methods.data?.google}
      <button type="button" class="button-secondary auth-passkey" disabled={busy} onclick={() => window.location.assign('/api/auth/google/start?next=' + encodeURIComponent(next))}>Continue with Google</button>
      <div class="auth-separator"><span>or</span></div>
    {/if}
    {#if stage === 'code'}<p class="auth-detail">Enter the 6-digit code sent to <strong>{email}</strong>.</p>{/if}
    {#if stage === 'username'}<p class="auth-detail">{email}</p>{/if}
    <form onsubmit={submit} class="auth-form">
      {#if stage === 'email'}
        <input class="text-input" bind:value={email} type="email" autocomplete="email" maxlength={254} required placeholder="Email" aria-label="Email" disabled={busy} />
      {:else if stage === 'code'}
        <input class="text-input otp-input" bind:value={code} inputmode="numeric" autocomplete="one-time-code" pattern={"[0-9]{6}"} minlength={6} maxlength={6} required placeholder="000000" aria-label="Verification code" disabled={busy} />
      {:else}
        <input class="text-input" bind:value={username} minlength={3} maxlength={32} pattern={"[a-z0-9_]{3,32}"} autocomplete="username" required placeholder="Username" aria-label="Username" disabled={busy} />
      {/if}
      {#if error}<p class="error-note" role="alert">{error}</p>{/if}
      <button class="button-primary auth-submit" disabled={busy || (stage === 'email' && methods.data?.email === false)}>{busy ? 'Please wait…' : stage === 'email' ? 'Send code' : stage === 'code' ? 'Verify code' : 'Continue'}</button>
      {#if stage === 'email' && methods.data?.email === false}<p class="auth-detail">Email sign-in is currently unavailable.</p>{/if}
    </form>
    {#if stage === 'code'}
      <button class="button-secondary auth-passkey" type="button" disabled={busy || retryIn > 0} onclick={() => send.mutate()}>{retryIn > 0 ? `Resend code in ${retryIn}s` : 'Resend code'}</button>
      <button class="auth-switch auth-back" type="button" disabled={busy} onclick={() => { stage = 'email'; code = ''; error = '' }}>Use a different email</button>
    {/if}
  </section>
</main>

<style>
  .auth-detail { color: var(--text-secondary, #64766e); overflow-wrap: anywhere; margin: 0 0 20px; line-height: 1.5; }
  .otp-input { text-align: center; letter-spacing: .35em; font-size: 24px; }
  .auth-back { background: none; border: none; width: 100%; cursor: pointer; }
  .auth-passkey { margin-top: 16px; }
</style>
