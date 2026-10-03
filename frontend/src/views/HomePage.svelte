<script lang="ts">
  import { createQuery } from '@tanstack/svelte-query'

  import Navigate from '../ui/Navigate.svelte'

  import { currentUser } from '../api'

  import Dashboard from './Dashboard.svelte'

  const userQuery = createQuery(() => ({
    queryKey: ['me'],
    queryFn: currentUser,
  }))
  const user = $derived(userQuery.data)
  const isLoading = $derived(userQuery.isLoading)
</script>

{#if isLoading}
  <main class="app-page">
    <div class="skeleton h-80"></div>
  </main>
{:else if !user}
  <Navigate to="/login" />
{:else if user.must_change_password}
  <Navigate to="/first-password" />
{:else}
  <Dashboard {user} />
{/if}
