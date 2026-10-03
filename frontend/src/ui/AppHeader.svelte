<script lang="ts">
  import {
    createQuery,
    createMutation,
    useQueryClient,
  } from '@tanstack/svelte-query'
  import { navigate, route } from '../navigation.svelte'

  import { LogOut } from '@lucide/svelte'
  import { api, currentUser } from '../api'
  import { initials } from '../utils'
  const pathname = $derived(route.pathname)
  const queryClient = useQueryClient()
  const userQuery = createQuery(() => ({
    queryKey: ['me'],
    queryFn: currentUser,
  }))
  const user = $derived(userQuery.data)
  let theme: Theme = $state(
    (() =>
      localStorage.getItem('mova-theme') === 'dark' ? 'dark' : 'light')(),
  )
  const logout = createMutation(() => ({
    mutationFn: () => api<void>('/api/auth/logout', { method: 'POST' }),
    onSuccess: async () => {
      await queryClient.cancelQueries()
      queryClient.setQueryData(['me'], null)
      queryClient.removeQueries({
        predicate: (query) => query.queryKey[0] !== 'me',
      })
      await navigate({ to: '/login' })
    },
  }))
  $effect(() => {
    document.documentElement.dataset.theme = theme
    localStorage.setItem('mova-theme', theme)
  })

  type Theme = 'light' | 'dark'
</script>

{#if user && !user.must_change_password && !pathname.startsWith('/r/') && pathname !== '/login' && pathname !== '/first-password'}
  <aside class="app-sidebar">
    <a href="/" class="brand" aria-label="Mowa — друзья">
      <span class="brand-dot" aria-hidden="true"></span>
      <span>mowa</span>
    </a>

    <nav class="sidebar-nav" aria-label="Основная навигация">
      <a href="/" class={`sidebar-link ${pathname === '/' ? 'active' : ''}`}
        >Друзья</a
      >
      <a
        href="/settings"
        class={`sidebar-link ${pathname === '/settings' ? 'active' : ''}`}
        >Настройки</a
      >
    </nav>

    <div class="sidebar-spacer"></div>
    <div class="theme-switch" aria-label="Цветовая тема">
      <button
        class={theme === 'light' ? 'active' : ''}
        onclick={() => (theme = 'light')}>Светлая</button
      >
      <button
        class={theme === 'dark' ? 'active' : ''}
        onclick={() => (theme = 'dark')}>Тёмная</button
      >
    </div>

    <div class="sidebar-profile">
      <span class="sidebar-avatar">{initials(user.display_name)}<i></i></span>
      <span class="sidebar-user"
        ><strong>{user.display_name}</strong><small
          >@{user.username} · в сети</small
        ></span
      >
      <button
        class="sidebar-logout"
        onclick={() => logout.mutate()}
        aria-label="Выйти"
        title="Выйти"><LogOut size={17} /></button
      >
    </div>
  </aside>
{/if}
