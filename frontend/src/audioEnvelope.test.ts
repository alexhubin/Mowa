import { expect, it } from 'vitest'
import { AudioEnvelope } from './audioEnvelope'
it('fades down, holds across pauses and recovers gradually', () => {
  const envelope = new AudioEnvelope()
  expect(envelope.update(true, false, 70, 0)).toBe(1)
  const attack = envelope.update(true, true, 70, 40)
  expect(attack).toBeGreaterThan(0.3)
  expect(attack).toBeLessThan(1)
  const held = envelope.update(true, false, 70, 400)
  expect(held).toBeLessThan(attack)
  const release = envelope.update(true, false, 70, 560)
  expect(release).toBeGreaterThan(held)
  expect(release).toBeLessThan(1)
  expect(envelope.update(true, false, 70, 3000)).toBe(1)
})
it('disabling releases softly even while the author is speaking', () => {
  const envelope = new AudioEnvelope()
  envelope.update(true, true, 100, 0)
  expect(envelope.update(true, true, 100, 800)).toBe(0)
  const released = envelope.update(false, true, 100, 840)
  expect(released).toBeGreaterThan(0)
  expect(released).toBeLessThan(1)
  expect(envelope.update(false, true, 100, 3000)).toBe(1)
})
