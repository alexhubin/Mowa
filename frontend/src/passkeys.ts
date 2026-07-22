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
    throw friendlyPasskeyError(error, 'Не удалось добавить passkey')
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
    throw friendlyPasskeyError(error, 'Не удалось войти по passkey')
  }
}

function ensurePasskeysSupported() {
  if (!passkeysSupported()) throw new Error('Этот браузер или соединение не поддерживает passkey')
}

function friendlyPasskeyError(error: unknown, fallback: string) {
  if (!(error instanceof Error)) return new Error(fallback)
  if (error.name === 'NotAllowedError') return new Error('Операция отменена или время подтверждения истекло')
  if (error.name === 'InvalidStateError') return new Error('Этот passkey уже добавлен')
  if (error.name === 'SecurityError') return new Error('Passkey недоступен для этого домена')
  return error
}
