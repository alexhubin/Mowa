export type User = {
  id: string
  username: string
  email: string
  display_name: string
  must_change_password: boolean
}

export type RoomInfo = {
  id: string
  invite_code: string
  name: string
  owner_id: string
  kind: 'group' | 'direct'
  created_at: string
}

export type RoomToken = {
  token: string
  server_url: string
  expires_in: number
}

export type RoomMessage = {
  id: string
  body: string
  author: {
    id: string
    username: string
    display_name: string
  }
  created_at: string
}

export type FriendUser = {
  id: string
  username: string
  display_name: string
  relationship?: 'none' | 'friends' | 'request_sent' | 'request_received'
  online: boolean
}

export type FriendRequest = {
  id: string
  user: FriendUser
  created_at: string
}

export type FriendsPayload = {
  friends: FriendUser[]
  incoming: FriendRequest[]
  outgoing: FriendRequest[]
}

export type DirectCall = {
  id: string
  status: 'ringing' | 'active'
  invite_code: string
  peer: FriendUser
  incoming: boolean
  created_at: string
}

export type AccountSettings = {
  video_quality: 'low' | 'high'
}

export class ApiError extends Error {
  constructor(
    message: string,
    public status: number,
  ) {
    super(message)
  }
}

export async function api<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(path, {
    credentials: 'same-origin',
    ...init,
    headers: {
      ...(init?.body ? { 'Content-Type': 'application/json' } : {}),
      ...init?.headers,
    },
  })

  if (!response.ok) {
    let message = 'Что-то пошло не так'
    try {
      const body = (await response.json()) as { error?: string }
      if (body.error) message = body.error
    } catch {
      // Preserve the friendly fallback for non-JSON proxy errors.
    }
    throw new ApiError(message, response.status)
  }

  if (response.status === 204) return undefined as T
  return (await response.json()) as T
}

export async function currentUser(): Promise<User | null> {
  try {
    return await api<User>('/api/auth/me')
  } catch (error) {
    if (error instanceof ApiError && error.status === 401) return null
    throw error
  }
}
