<script lang="ts">
  import { createMutation, useQueryClient } from '@tanstack/svelte-query'
  import { navigate } from '../navigation.svelte'

  import { KeyRound } from '@lucide/svelte'
  import { api, type User } from '../api'
  import { loginWithPasskey, passkeysSupported } from '../passkeys'
  const queryClient = useQueryClient()
  let email = $state('')
  let password = $state('')
  async function completeLogin(user: User) {
    queryClient.setQueryData(['me'], user)
    const next = new URLSearchParams(window.location.search).get('next')
    if (user.must_change_password) await navigate({ to: '/first-password' })
    else if (next?.startsWith('/') && !next.startsWith('//'))
      window.location.assign(next)
    else await navigate({ to: '/' })
  }
  const mutation = createMutation(() => ({
    mutationFn: () =>
      api<User>('/api/auth/login', {
        method: 'POST',
        body: JSON.stringify({ email, password }),
      }),
    onSuccess: completeLogin,
  }))
  const passkey = createMutation(() => ({
    mutationFn: loginWithPasskey,
    onSuccess: completeLogin,
  }))
  function submit(event: SubmitEvent) {
    event.preventDefault()
    mutation.mutate()
  }
</script>

<main class="auth-screen">
  <section class="auth-card">
    <div class="brand auth-brand">
      <span class="brand-dot" aria-hidden="true"></span><span>mowa</span>
    </div>
    <p class="auth-lead">
      Голос и экран — для своих.<br />Без серверов, каналов и лишнего.
    </p>
    <form onsubmit={submit} class="auth-form">
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
        autocomplete="current-password"
        required
        placeholder="Пароль"
        aria-label="Пароль"
      />
      {#if mutation.error}
        <p class="error-note" role="alert">
          {mutation.error.message}
        </p>
      {/if}
      <button class="button-primary auth-submit" disabled={mutation.isPending}
        >{mutation.isPending ? 'Минутку…' : 'Войти'}</button
      >
    </form>
    {#if passkeysSupported()}
      <div class="auth-separator"><span>или</span></div>
      <button
        type="button"
        class="button-secondary auth-passkey"
        onclick={() => passkey.mutate()}
        disabled={passkey.isPending || mutation.isPending}
      >
        <KeyRound size={18} />
        {passkey.isPending ? 'Подтвердите на устройстве…' : 'Войти по passkey'}
      </button>
      {#if passkey.error}
        <p class="error-note auth-passkey-error" role="alert">
          {passkey.error.message}
        </p>
      {/if}
    {/if}
    <p class="auth-footnote">Аккаунты создаёт администратор Mowa</p>
  </section>
</main>
