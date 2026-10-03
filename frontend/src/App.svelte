<script lang="ts">
  import { QueryClient, QueryClientProvider } from '@tanstack/svelte-query'
  import { route, syncLocation, interceptLink } from './navigation.svelte'
  import AppHeader from './ui/AppHeader.svelte'
  import IncomingCall from './ui/IncomingCall.svelte'
  import HomePage from './views/HomePage.svelte'
  import AuthPage from './views/AuthPage.svelte'
  import FirstPasswordPage from './views/FirstPasswordPage.svelte'
  import SettingsPage from './views/SettingsPage.svelte'
  import RoomPage from './views/RoomPage.svelte'
  import NotFoundPage from './views/NotFoundPage.svelte'

  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { staleTime: 30_000, retry: 1, refetchOnWindowFocus: false },
      mutations: { retry: 0 },
    },
  })
  const roomMatch = $derived(/^\/r\/([^/]+)\/?$/.exec(route.pathname))
  const inviteCode = $derived.by(() => {
    try {
      return roomMatch ? decodeURIComponent(roomMatch[1]) : null
    } catch {
      return null
    }
  })
</script>

<svelte:window onpopstate={syncLocation} onclick={interceptLink} />
<QueryClientProvider client={queryClient}>
  <div class="min-h-dvh">
    <AppHeader />
    {#if route.pathname === '/'}
      <HomePage />
    {:else if route.pathname === '/login'}
      <AuthPage />
    {:else if route.pathname === '/first-password'}
      <FirstPasswordPage />
    {:else if route.pathname === '/settings'}
      <SettingsPage />
    {:else if inviteCode}{#key inviteCode}
        <RoomPage {inviteCode} />
      {/key}
    {:else}
      <NotFoundPage />
    {/if}
    <IncomingCall />
  </div>
</QueryClientProvider>
