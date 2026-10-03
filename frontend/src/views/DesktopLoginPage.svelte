<script lang="ts">
  import { createQuery, createMutation } from '@tanstack/svelte-query'
  import { api, currentUser } from '../api'
  import Navigate from '../ui/Navigate.svelte'
  const id = new URLSearchParams(window.location.search).get('request') ?? ''
  const next = '/desktop-login?request=' + encodeURIComponent(id)
  const user = createQuery(() => ({queryKey: ['me'], queryFn: currentUser}))
  const approve = createMutation(() => ({
    mutationFn: () => api<void>('/api/auth/desktop/approve', {method: 'POST', body: JSON.stringify({id})}),
  }))
</script>

{#if user.isPending}
  <main class="auth-screen"><div class="skeleton h-80 w-full max-w-md"></div></main>
{:else if !user.data}
  <Navigate to={'/login?next=' + encodeURIComponent(next)} />
{:else if user.data.must_change_password}
  <Navigate to={'/first-password?next=' + encodeURIComponent(next)} />
{:else}
  <main class="auth-screen">
    <section class="auth-card">
      <div class="brand auth-brand"><span class="brand-dot" aria-hidden="true"></span><span>mowa</span></div>
      {#if approve.isSuccess}
        <h1 class="first-password-title">Done</h1>
        <p class="auth-lead">Return to Mowa Desktop.</p>
      {:else if !/^[A-Za-z0-9_-]{43}$/.test(id)}
        <p class="error-note" role="alert">Start sign-in from Mowa Desktop.</p>
      {:else}
        <h1 class="first-password-title">Sign in to Mowa Desktop</h1>
        <p class="auth-lead">{user.data.display_name} · {user.data.email}</p>
        <p class="auth-footnote">Only approve if you opened this page from your own app.</p>
        {#if approve.error}<p class="error-note" role="alert">{approve.error.message}</p>{/if}
        <button class="button-primary auth-submit" disabled={approve.isPending} onclick={() => approve.mutate()}>Sign in to app</button>
      {/if}
    </section>
  </main>
{/if}
