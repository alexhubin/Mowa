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
  const rawNext = new URLSearchParams(window.location.search).get('next')
  const next = rawNext?.startsWith('/') && !rawNext.startsWith('//') && !rawNext.includes('\\') ? rawNext : '/'
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
      await navigate({ to: next })
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
  <Navigate to={next} />
{:else}
  <main class="auth-screen">
    <section class="auth-card">
      <div class="brand auth-brand">
        <span class="brand-dot" aria-hidden="true"></span><span>mowa</span>
      </div>
      <h1 class="first-password-title">Choose a new password</h1>
      <p class="auth-lead">
        Replace your temporary password.
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
          placeholder="New password"
          aria-label="New password"
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
          placeholder="Confirm password"
          aria-label="Confirm password"
        />
        {#if mismatch}
          <p class="inline-error">Passwords do not match</p>
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
            ? 'Saving…'
            : 'Save and continue'}</button
        >
      </form>
      <p class="auth-footnote">At least 8 characters</p>
    </section>
  </main>
{/if}
