import { describe, expect, it } from 'vitest'
import { initials } from './utils'

describe('initials', () => {
  it('uses up to two name parts', () => {
    expect(initials('Анна Смирнова')).toBe('АС')
    expect(initials('Mowa')).toBe('M')
  })
})

