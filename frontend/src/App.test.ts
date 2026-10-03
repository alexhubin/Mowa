import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/svelte'
import App from './App.svelte'
import { navigate, route } from './navigation.svelte'
import type { DirectCall, RoomMessage, User } from './api'

class MockEventSource extends EventTarget {
  static instances: MockEventSource[] = []
  close = vi.fn()
  constructor(public url: string) {
    super()
    MockEventSource.instances.push(this)
  }
}

const user: User = {
  id: 'me',
  username: 'alex',
  display_name: 'Алекс',
  email: 'alex@example.com',
  must_change_password: false,
}
const friend = {
  id: 'friend',
  username: 'sonya',
  display_name: 'Соня',
  online: true,
}
let session: User | null
let messages: RoomMessage[]
let calls: DirectCall[]
let fetchMock: ReturnType<typeof vi.fn>

beforeEach(() => {
  session = { ...user }
  messages = []
  calls = []
  localStorage.clear()
  history.replaceState(null, '', '/')
  route.pathname = '/'
  MockEventSource.instances = []
  vi.stubGlobal('EventSource', MockEventSource)
  Element.prototype.scrollIntoView = vi.fn()
  fetchMock = vi.fn(async (input: string, init?: RequestInit) => {
    const method = init?.method ?? 'GET'
    const body = init?.body ? JSON.parse(String(init.body)) : {}
    const json = (value: unknown, status = 200) =>
      new Response(JSON.stringify(value), {
        status,
        headers: { 'Content-Type': 'application/json' },
      })
    if (input === '/api/auth/me')
      return session ? json(session) : json({ error: 'Unauthorized' }, 401)
    if (input === '/api/auth/login' || input === '/api/auth/register') {
      session = { ...user }
      return json(session)
    }
    if (input === '/api/auth/desktop/approve') return new Response(null, {status:204})
    if (input === '/api/auth/logout') {
      session = null
      return new Response(null, { status: 204 })
    }
    if (input === '/api/auth/first-password') {
      session = { ...user }
      return new Response(null, { status: 204 })
    }
    if (input === '/api/friends')
      return json({ friends: [friend], incoming: [], outgoing: [] })
    if (input.startsWith('/api/users/search?'))
      return json([{ ...friend, relationship: 'none' }])
    if (input === '/api/friend-requests') return json({})
    if (input === '/api/calls') return json(calls)
    if (input === '/api/calls/call-1/accept') return json(calls[0])
    if (input === '/api/account/settings')
      return json({ video_quality: 'high' })
    if (input === '/api/account/passkeys') return json([])
    if (input === '/api/presence') return new Response(null, { status: 204 })
    if (input === '/api/direct-messages/friend') {
      if (method === 'POST') {
        const message = {
          id: 'message-1',
          body: body.body,
          author: user,
          created_at: new Date().toISOString(),
        }
        messages.push(message)
        return json(message)
      }
      return json(messages)
    }
    if (input.startsWith('/api/rooms/'))
      return json({
        id: 'room-1',
        invite_code: input.split('/')[3],
        name: 'Тестовая комната',
        owner_id: user.id,
        kind: 'group',
      })
    throw new Error(`Unexpected request: ${method} ${input}`)
  })
  vi.stubGlobal('fetch', fetchMock)
})

afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
})

const requestsTo = (path: string) =>
  fetchMock.mock.calls.filter(([url]) => url === path)

describe('Svelte application with TanStack Query', () => {
  it('shares the current-user query and reacts to search keys and friend mutations', async () => {
    render(App)
    await screen.findByRole('heading', { name: 'Friends' })
    expect(requestsTo('/api/auth/me')).toHaveLength(1)
    const search = screen.getByRole('textbox', { name: 'Find a friend' })
    await fireEvent.input(search, { target: { value: 'sonya' } })
    await screen.findByRole('button', { name: 'Add' })
    expect(requestsTo('/api/users/search?q=sonya')).toHaveLength(1)
    await fireEvent.click(screen.getByRole('button', { name: 'Add' }))
    await waitFor(() => expect(requestsTo('/api/friends')).toHaveLength(2))
    await fireEvent.input(search, { target: { value: 'alex' } })
    await waitFor(() =>
      expect(requestsTo('/api/users/search?q=alex')).toHaveLength(1),
    )
  })

  it('redirects unauthenticated users and updates the shared cache after login', async () => {
    session = null
    render(App)
    const email = await screen.findByRole('textbox', { name: 'Email' })
    expect(route.pathname).toBe('/login')
    await fireEvent.input(email, { target: { value: user.email } })
    await fireEvent.input(screen.getByLabelText('Password', { exact: true }), {
      target: { value: 'password123' },
    })
    await fireEvent.submit(email.closest('form')!)
    await screen.findByRole('heading', { name: 'Friends' })
    expect(route.pathname).toBe('/')
    expect(requestsTo('/api/auth/login')).toHaveLength(1)
    await fireEvent.click(screen.getByRole('button', { name: 'Sign out' }))
    await screen.findByRole('textbox', { name: 'Email' })
    await waitFor(() =>
      expect(
        MockEventSource.instances.every(
          (source) => source.close.mock.calls.length > 0,
        ),
      ).toBe(true),
    )
  })

  it('enforces first-password setup and opens the dashboard after saving', async () => {
    session = { ...user, must_change_password: true }
    render(App)
    const password = await screen.findByLabelText('New password')
    expect(route.pathname).toBe('/first-password')
    expect(requestsTo('/api/friends')).toHaveLength(0)
    await fireEvent.input(password, { target: { value: 'new-password123' } })
    await fireEvent.input(screen.getByLabelText('Confirm password'), {
      target: { value: 'different123' },
    })
    expect(
      screen.getByRole('button', { name: 'Save and continue' }),
    ).toBeDisabled()
    await fireEvent.input(screen.getByLabelText('Confirm password'), {
      target: { value: 'new-password123' },
    })
    await fireEvent.submit(password.closest('form')!)
    await screen.findByRole('heading', { name: 'Friends' })
  })

  it('sends direct messages, refreshes on SSE and closes the subscription with the dialog', async () => {
    render(App)
    await fireEvent.click(
      await screen.findByRole('button', { name: 'Message Соня' }),
    )
    const composer = screen.getByRole('textbox', { name: 'Message' })
    await fireEvent.input(composer, { target: { value: 'Привет!' } })
    await fireEvent.submit(composer.closest('form')!)
    await screen.findByText('Привет!')
    expect(composer).toHaveValue('')
    messages.push({
      id: 'message-2',
      body: 'Ответ через SSE',
      author: friend,
      created_at: new Date().toISOString(),
    })
    const source = MockEventSource.instances.find(
      (source) => source.url === '/api/direct-messages/friend/events',
    )!
    source.dispatchEvent(new Event('messages'))
    await screen.findByText('Ответ через SSE')
    await fireEvent.click(
      screen.getByRole('button', { name: 'Close chat' }),
    )
    expect(source.close).toHaveBeenCalledOnce()
  })

  it('updates incoming calls from SSE and navigates to the accepted room', async () => {
    calls = [
      {
        id: 'initial',
        status: 'ringing',
        invite_code: 'MOWA-OLD',
        peer: friend,
        incoming: false,
        created_at: new Date().toISOString(),
      },
    ]
    render(App)
    await screen.findByText(/Calling/)
    await screen.findByRole('heading', { name: 'Friends' })
    calls = [
      {
        id: 'call-1',
        status: 'ringing',
        invite_code: 'MOWA-1234',
        peer: friend,
        incoming: true,
        created_at: new Date().toISOString(),
      },
    ]
    MockEventSource.instances
      .find(
        (source) =>
          source.url === '/api/calls/events' &&
          source.close.mock.calls.length === 0,
      )!
      .dispatchEvent(new Event('calls'))
    await fireEvent.click(
      await screen.findByRole('button', { name: 'Accept' }),
    )
    await screen.findByText('Ready to join?')
    expect(route.pathname).toBe('/r/MOWA-1234')
  })

  it('supports internal links, browser history, room deep links and unknown routes', async () => {
    render(App)
    await fireEvent.click(
      await screen.findByRole('link', { name: 'Settings' }),
    )
    await screen.findByRole('heading', { name: 'Settings' })
    history.replaceState(null, '', '/')
    window.dispatchEvent(new PopStateEvent('popstate'))
    await screen.findByRole('heading', { name: 'Friends' })
    navigate({ to: '/r/$inviteCode', params: { inviteCode: 'MOWA-DEEP' } })
    await screen.findByText('Ready to join?')
    expect(requestsTo('/api/rooms/MOWA-DEEP')).toHaveLength(1)
    navigate({ to: '/missing' })
    await screen.findByText('This page or room does not exist.')
  })
})

describe('Registration and browser desktop login', () => {
  it('registers an account without displaying quotas', async () => {
    session = null
    navigate({to:'/register'})
    render(App)
    await screen.findByRole('heading',{name:'Sign up'})
    await fireEvent.input(screen.getByLabelText('Username'),{target:{value:'alex'}})
    await fireEvent.input(screen.getByLabelText('Email'),{target:{value:'alex@example.com'}})
    await fireEvent.input(screen.getByLabelText('Password'),{target:{value:'secure-password'}})
    await fireEvent.input(screen.getByLabelText('Confirm password'),{target:{value:'different-password'}})
    expect(screen.getByRole('button',{name:'Create account'})).toBeDisabled()
    await fireEvent.input(screen.getByLabelText('Confirm password'),{target:{value:'secure-password'}})
    expect(screen.queryByText(/лимит|10 аккаунтов/i)).not.toBeInTheDocument()
    await fireEvent.submit(screen.getByLabelText('Email').closest('form')!)
    await screen.findByRole('heading',{name:'Friends'})
    expect(requestsTo('/api/auth/register')).toHaveLength(1)
  })
  it('requires an explicit approval before signing into desktop', async () => {
    const id='a'.repeat(43)
    navigate({to:'/desktop-login?request='+id})
    render(App)
    await screen.findByRole('button',{name:'Sign in to app'})
    expect(requestsTo('/api/auth/desktop/approve')).toHaveLength(0)
    await fireEvent.click(screen.getByRole('button',{name:'Sign in to app'}))
    await screen.findByRole('heading',{name:'Done'})
    expect(requestsTo('/api/auth/desktop/approve')).toHaveLength(1)
  })
  it('preserves the desktop request through login and registration navigation', async () => {
    session=null
    const next='/desktop-login?request='+'a'.repeat(43)
    navigate({to:next})
    render(App)
    await screen.findByRole('heading',{name:'Sign in'})
    expect(new URLSearchParams(location.search).get('next')).toBe(next)
    await fireEvent.click(screen.getByRole('link',{name:'Create account'}))
    await screen.findByRole('heading',{name:'Sign up'})
    expect(new URLSearchParams(location.search).get('next')).toBe(next)
  })
})
