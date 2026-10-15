import type { SessionStatus } from './types';

export interface SessionToken { epoch: number; vaultId: string }
// Reject late secrets after a lock, vault switch, disconnect or newer metadata observation.
export class SessionGuard {
  private epoch = 0;
  private sequence = 0;
  private observed = 0;
  private state?: SessionStatus;
  probe(): number { return ++this.sequence; }
  observe(state: SessionStatus, sequence: number): { accepted: boolean; invalidated: boolean; metadataChanged: boolean } {
    if (sequence < this.observed) return { accepted: false, invalidated: false, metadataChanged: false };
    this.observed = sequence;
    const invalidated = !this.state || this.state.vaultId !== state.vaultId || this.state.locked !== state.locked || this.state.configured !== state.configured;
    const metadataChanged = !!this.state && this.state.entriesVersion !== state.entriesVersion;
    if (invalidated || metadataChanged) this.epoch++;
    this.state = state;
    return { accepted: true, invalidated, metadataChanged };
  }
  disconnect(): void { this.epoch++; this.observed = ++this.sequence; this.state = undefined; }
  capture(): SessionToken { return { epoch: this.epoch, vaultId: this.state?.vaultId ?? '' }; }
  valid(token: SessionToken): boolean { return !!this.state?.configured && !this.state.locked && token.epoch === this.epoch && token.vaultId === this.state.vaultId; }
}
