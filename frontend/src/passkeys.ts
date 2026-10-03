import {
  browserSupportsWebAuthn,
  startAuthentication,
  startRegistration,
  type PublicKeyCredentialCreationOptionsJSON,
  type PublicKeyCredentialRequestOptionsJSON,
} from '@simplewebauthn/browser'
import { api, type User } from './api'

export type Passkey = {
  id: string
  name: string
  created_at: string
  last_used_at?: string
}

type RegistrationOptions = { publicKey: PublicKeyCredentialCreationOptionsJSON }
type AuthenticationOptions = { publicKey: PublicKeyCredentialRequestOptionsJSON }

export function passkeysSupported() {
  return window.isSecureContext && browserSupportsWebAuthn()
}

export async function registerPasskey(name: string) {
  ensurePasskeysSupported()
  try {
    const options = await api<RegistrationOptions>('/api/account/passkeys/register/begin', {
      method: 'POST',
      body: JSON.stringify({ name }),
    })
    const credential = await startRegistration({ optionsJSON: options.publicKey })
    return await api<Passkey>('/api/account/passkeys/register/finish', {
      method: 'POST',
      body: JSON.stringify(credential),
    })
  } catch (error) {
    throw friendlyPasskeyError(error, 'Could not add passkey')
  }
}

export async function loginWithPasskey() {
  ensurePasskeysSupported()
  try {
    const options = await api<AuthenticationOptions>('/api/auth/passkey/login/begin', { method: 'POST' })
    const credential = await startAuthentication({ optionsJSON: options.publicKey })
    return await api<User>('/api/auth/passkey/login/finish', {
      method: 'POST',
      body: JSON.stringify(credential),
    })
  } catch (error) {
    throw friendlyPasskeyError(error, 'Could not sign in with a passkey')
  }
}

function ensurePasskeysSupported() {
  if (!passkeysSupported()) throw new Error('This browser or connection does not support passkeys')
}

function friendlyPasskeyError(error: unknown, fallback: string) {
  if (!(error instanceof Error)) return new Error(fallback)
  if (error.name === 'NotAllowedError') return new Error('Request cancelled or timed out')
  if (error.name === 'InvalidStateError') return new Error('This passkey has already been added')
  if (error.name === 'SecurityError') return new Error('Passkeys are unavailable for this domain')
  return error
}
