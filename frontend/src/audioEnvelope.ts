// Time-based envelope: fast soft attack, hold across syllables, slower recovery.
export class AudioEnvelope {
  private value = 1
  private last?: number
  private holdUntil = 0
  update(enabled: boolean, speaking: boolean, reduction: number, now: number): number {
    if (enabled && speaking) this.holdUntil = now + 500
    if (!enabled) this.holdUntil = 0
    const target = enabled && (speaking || now < this.holdUntil) ? 1 - reduction / 100 : 1
    const elapsed = this.last === undefined ? 0 : Math.max(0, now - this.last)
    this.last = now
    const timeConstant = target < this.value ? 80 : 250
    this.value = target + (this.value - target) * Math.exp(-elapsed / timeConstant)
    if (Math.abs(this.value - target) < 0.001) this.value = target
    return this.value
  }
}
