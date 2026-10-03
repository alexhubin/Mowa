<script lang="ts">
  import { createMutation, useQueryClient } from '@tanstack/svelte-query'
  import { navigate, route } from '../navigation.svelte'

  import { KeyRound } from '@lucide/svelte'
  import { api, type User } from '../api'
  import { loginWithPasskey, passkeysSupported } from '../passkeys'
  const queryClient = useQueryClient()
  const registering = $derived(route.pathname === '/register')
  let username = $state('')
  let confirmation = $state('')
  let email = $state('')
  let password = $state('')
  async function completeLogin(user: User) {
    queryClient.setQueryData(['me'], user)
    const next = new URLSearchParams(window.location.search).get('next')
    if (user.must_change_password) await navigate({ to: '/first-password' + (next ? '?next=' + encodeURIComponent(next) : '') })
    else if (next?.startsWith('/') && !next.startsWith('//') && !next.includes('\\'))
      window.location.assign(next)
    else await navigate({ to: '/' })
  }
  const mutation = createMutation(() => ({
    mutationFn: () =>
      api<User>(registering ? '/api/auth/register' : '/api/auth/login', {
        method: 'POST',
        body: JSON.stringify(registering ? { email, password, username } : { email, password }),
      }),
    onSuccess: completeLogin,
  }))
  const passkey = createMutation(() => ({
    mutationFn: loginWithPasskey,
    onSuccess: completeLogin,
  }))
  function submit(event: SubmitEvent) {
    event.preventDefault()
    if (!registering || password === confirmation) mutation.mutate()
  }
</script>

<main class="auth-screen">
  <section class="auth-card">
    <div class="brand auth-brand">
      <span class="brand-dot" aria-hidden="true"></span><span>mowa</span>
    </div>
    <h1 class="first-password-title">{registering ? 'Sign up' : 'Sign in'}</h1>
    <form onsubmit={submit} class="auth-form">
      {#if registering}
        <input class="text-input" bind:value={username} minlength={3} maxlength={32} pattern={"[a-z0-9_]{3,32}"} required placeholder="Username" aria-label="Username" autocomplete="username" />
      {/if}
      <input
        class="text-input"
        value={email}
        oninput={(event) => (email = event.currentTarget.value)}
        type="email"
        autocomplete="email"
        required
        placeholder="Email"
        aria-label="Email"
      />
      <input
        class="text-input"
        value={password}
        oninput={(event) => (password = event.currentTarget.value)}
        type="password"
        minlength={8}
        maxlength={128}
        autocomplete={registering ? "new-password" : "current-password"}
        required
        placeholder="Password"
        aria-label="Password"
      />
      {#if registering}
        <input class="text-input" type="password" bind:value={confirmation} required minlength={8} maxlength={128} placeholder="Confirm password" aria-label="Confirm password" autocomplete="new-password" />
        {#if confirmation && password !== confirmation}<p class="error-note">Passwords do not match</p>{/if}
      {/if}
      {#if mutation.error}
        <p class="error-note" role="alert">
          {mutation.error.message}
        </p>
      {/if}
      <button class="button-primary auth-submit" disabled={mutation.isPending || (registering && password !== confirmation)}
        >{mutation.isPending ? 'Please wait…' : registering ? 'Create account' : 'Sign in'}</button
      >
    </form>
    {#if !registering && passkeysSupported()}
      <div class="auth-separator"><span>or</span></div>
      <button
        type="button"
        class="button-secondary auth-passkey"
        onclick={() => passkey.mutate()}
        disabled={passkey.isPending || mutation.isPending}
      >
        <KeyRound size={18} />
        {passkey.isPending ? 'Confirm on your device…' : 'Sign in with a passkey'}
      </button>
      {#if passkey.error}
        <p class="error-note auth-passkey-error" role="alert">
          {passkey.error.message}
        </p>
      {/if}
    {/if}
    <a class="auth-switch" href={(registering ? '/login' : '/register') + window.location.search} onclick={() => {mutation.reset(); passkey.reset()}}>{registering ? 'Already have an account? Sign in' : 'Create account'}</a>
  </section>
</main>
