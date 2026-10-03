<script lang="ts">
  import { focusOnMount } from '../focus'

  import {
    createQuery,
    createMutation,
    useQueryClient,
  } from '@tanstack/svelte-query'
  import { navigate } from '../navigation.svelte'

  import { ArrowUpRight, Check, X } from '@lucide/svelte'
  import {
    api,
    type DirectCall,
    type FriendsPayload,
    type FriendUser,
    type RoomInfo,
    type User,
  } from '../api'

  import DirectChat from './DirectChat.svelte'
  import FriendRow from './FriendRow.svelte'
  import SearchRow from './SearchRow.svelte'
  import Avatar from './Avatar.svelte'
  import UserLabel from './UserLabel.svelte'
  import EmptyList from './EmptyList.svelte'
  let { user }: { user: User } = $props()
  const queryClient = useQueryClient()
  let search = $state('')
  let joinOpen = $state(false)
  let joinValue = $state('')
  let chatFriend: FriendUser | null = $state(null)
  const friends = createQuery(() => ({
    queryKey: ['friends'],
    queryFn: () => api<FriendsPayload>('/api/friends'),
    refetchInterval: 30_000,
  }))
  const calls = createQuery(() => ({
    queryKey: ['calls'],
    queryFn: () => api<DirectCall[]>('/api/calls'),
  }))
  const searchQuery = createQuery(() => ({
    queryKey: ['user-search', search],
    queryFn: () =>
      api<FriendUser[]>(
        `/api/users/search?q=${encodeURIComponent(search.trim())}`,
      ),
    enabled: search.trim().length >= 2,
  }))
  const refreshFriends = () =>
    Promise.all([
      queryClient.invalidateQueries({ queryKey: ['friends'] }),
      queryClient.invalidateQueries({ queryKey: ['user-search'] }),
    ])
  const sendRequest = createMutation(() => ({
    mutationFn: (target: FriendUser) =>
      api('/api/friend-requests', {
        method: 'POST',
        body: JSON.stringify({ username: target.username }),
      }),
    onSuccess: refreshFriends,
  }))
  const acceptRequest = createMutation(() => ({
    mutationFn: (id: string) =>
      api(`/api/friend-requests/${id}/accept`, { method: 'POST' }),
    onSuccess: refreshFriends,
  }))
  const declineRequest = createMutation(() => ({
    mutationFn: (id: string) =>
      api(`/api/friend-requests/${id}`, { method: 'DELETE' }),
    onSuccess: refreshFriends,
  }))
  const startCall = createMutation(() => ({
    mutationFn: (friend: FriendUser) =>
      api<DirectCall>('/api/calls', {
        method: 'POST',
        body: JSON.stringify({ user_id: friend.id }),
      }),
    onSuccess: async (call) => {
      await queryClient.invalidateQueries({ queryKey: ['calls'] })
      await navigate({
        to: '/r/$inviteCode',
        params: { inviteCode: call.invite_code },
      })
    },
  }))
  const createRoom = createMutation(() => ({
    mutationFn: () =>
      api<RoomInfo>('/api/rooms', {
        method: 'POST',
        body: JSON.stringify({ name: `${user.display_name}'s room` }),
      }),
    onSuccess: (room) =>
      navigate({
        to: '/r/$inviteCode',
        params: { inviteCode: room.invite_code },
      }),
  }))
  const outgoing = $derived(
    calls.data?.find((call) => !call.incoming && call.status === 'ringing'),
  )
  const requests = $derived(friends.data?.incoming ?? [])
  function joinRoom(event: SubmitEvent) {
    event.preventDefault()
    const raw = joinValue.trim().replace(/\/+$/, '')
    const code = raw.split('/').pop()?.trim()
    if (!code) return
    void navigate({ to: '/r/$inviteCode', params: { inviteCode: code } })
  }
</script>

<main class="app-page friends-page">
  <header class="page-heading">
    <h1>Friends</h1>
    <div class="heading-actions">
      <button class="button-secondary" onclick={() => (joinOpen = true)}
        >Join by code</button
      >
      <button
        class="button-primary"
        onclick={() => createRoom.mutate()}
        disabled={createRoom.isPending}
        >Create room <ArrowUpRight size={17} /></button
      >
    </div>
  </header>

  <div class="friend-search-form">
    <input
      value={search}
      oninput={(event) => (search = event.currentTarget.value)}
      placeholder="Find a friend by username, e.g. sonya"
      aria-label="Find a friend"
    />
    <span class="search-action-label">Choose a person below</span>
  </div>
  {#if createRoom.error || startCall.error || sendRequest.error}
    <p class="error-note dashboard-error">
      {createRoom.error?.message ||
        startCall.error?.message ||
        sendRequest.error?.message}
    </p>
  {/if}

  {#if search.trim().length >= 2}
    <section class="search-results" aria-label="Search results">
      {#if searchQuery.isLoading}
        <div class="skeleton h-16"></div>
      {/if}
      {#each searchQuery.data ?? [] as person (person.id)}
        <SearchRow
          {person}
          onAdd={() => sendRequest.mutate(person)}
          busy={sendRequest.isPending}
        />
      {/each}
      {#if searchQuery.data?.length === 0}
        <EmptyList text="No results." />
      {/if}
    </section>
  {/if}

  {#if outgoing}
    <button
      class="outgoing-call"
      onclick={() =>
        navigate({
          to: '/r/$inviteCode',
          params: { inviteCode: outgoing.invite_code },
        })}
      ><span class="live-dot"></span><span
        >Calling <strong>{outgoing.peer.display_name}</strong>…</span
      ><ArrowUpRight size={18} /></button
    >
  {/if}

  {#if requests.length > 0}
    <section class="friends-section requests-section">
      <h2>Friend requests <span>{requests.length}</span></h2>
      <div class="request-list">
        {#each requests ?? [] as request (request.id)}
          <article class="request-card">
            <Avatar name={request.user.display_name} />
            <UserLabel user={request.user} detail="sent you a friend request" />
            <button
              class="button-primary compact"
              onclick={() => acceptRequest.mutate(request.id)}
              ><Check size={16} /> Accept</button
            >
            <button
              class="button-secondary compact"
              onclick={() => declineRequest.mutate(request.id)}
              ><X size={16} /> Decline</button
            >
          </article>
        {/each}
      </div>
    </section>
  {/if}

  <section class="friends-section">
    <h2>All friends <span>{friends.data?.friends.length ?? 0}</span></h2>
    <div class="friends-table">
      {#if friends.isLoading}
        <div class="skeleton h-40"></div>
      {/if}
      {#each friends.data?.friends ?? [] as friend (friend.id)}
        <FriendRow
          {friend}
          onMessage={() => (chatFriend = friend)}
          onCall={() => startCall.mutate(friend)}
          busy={startCall.isPending}
        />
      {/each}
      {#if friends.data?.friends.length === 0}
        <EmptyList text="Your friends will appear here." />
      {/if}
      <footer>
        You can message offline friends. Calls require them to be
        online.
      </footer>
    </div>
  </section>

  {#if joinOpen}
    <div
      class="modal-backdrop"
      role="presentation"
      onmousedown={(event) => {
        if (event.target === event.currentTarget) joinOpen = false
      }}
    >
      <form class="simple-modal" onsubmit={joinRoom}>
        <h2>Join a room</h2>
        <p>Paste a room code or invitation link.</p>
        <input
          class="text-input"
          use:focusOnMount
          value={joinValue}
          oninput={(event) => (joinValue = event.currentTarget.value)}
          placeholder="Room code or invitation link"
        />
        <div class="modal-actions">
          <button
            type="button"
            class="button-secondary"
            onclick={() => (joinOpen = false)}>Cancel</button
          ><button class="button-primary">Join</button>
        </div>
      </form>
    </div>
  {/if}

  {#if chatFriend}
    <DirectChat
      userID={user.id}
      friend={chatFriend}
      onClose={() => (chatFriend = null)}
    />
  {/if}
</main>
