<script lang="ts">
  import {
    createQuery,
    createMutation,
    useQueryClient,
  } from '@tanstack/svelte-query'
  import { navigate } from '../navigation.svelte'
  import Navigate from '../ui/Navigate.svelte'
  import { api, currentUser } from '../api'
  const queryClient = useQueryClient()
  const userQuery = createQuery(() => ({
    queryKey: ['me'],
    queryFn: currentUser,
  }))
  const user = $derived(userQuery.data)
  const isLoading = $derived(userQuery.isLoading)
  let password = $state('')
  let confirmation = $state('')
  const mismatch = $derived(
    confirmation.length > 0 && password !== confirmation,
  )
  const mutation = createMutation(() => ({
    mutationFn: () =>
      api<void>('/api/auth/first-password', {
        method: 'PUT',
        body: JSON.stringify({ new_password: password }),
      }),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ['me'] })
      await navigate({ to: '/' })
    },
  }))
  function submit(event: SubmitEvent) {
    event.preventDefault()
    if (!mismatch) mutation.mutate()
  }
</script>

{#if isLoading}
  <main class="auth-screen">
    <div class="skeleton h-80 w-full max-w-md"></div>
  </main>
{:else if !user}
  <Navigate to="/login" />
{:else if !user.must_change_password}
  <Navigate to="/" />
{:else}
  <main class="auth-screen">
    <section class="auth-card">
      <div class="brand auth-brand">
        <span class="brand-dot" aria-hidden="true"></span><span>mowa</span>
      </div>
      <h1 class="first-password-title">Придумайте новый пароль</h1>
      <p class="auth-lead">
        Это ваш первый вход — задайте свой пароль вместо временного.
      </p>
      <form class="auth-form" onsubmit={submit}>
        <input
          class="text-input"
          type="password"
          value={password}
          oninput={(event) => (password = event.currentTarget.value)}
          minlength={8}
          maxlength={128}
          autocomplete="new-password"
          required
          placeholder="Новый пароль"
          aria-label="Новый пароль"
        />
        <input
          class="text-input"
          type="password"
          value={confirmation}
          oninput={(event) => (confirmation = event.currentTarget.value)}
          minlength={8}
          maxlength={128}
          autocomplete="new-password"
          required
          placeholder="Повторите пароль"
          aria-label="Повторите пароль"
        />
        {#if mismatch}
          <p class="inline-error">Пароли не совпадают</p>
        {/if}
        {#if mutation.error}
          <p class="error-note" role="alert">
            {mutation.error.message}
          </p>
        {/if}
        <button
          class="button-primary auth-submit"
          disabled={mutation.isPending || mismatch}
          >{mutation.isPending
            ? 'Сохраняем…'
            : 'Сохранить и продолжить'}</button
        >
      </form>
      <p class="auth-footnote">Минимум 8 символов</p>
    </section>
  </main>
{/if}
