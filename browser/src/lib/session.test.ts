import { describe, expect, it } from 'vitest';
import { SessionGuard } from './session';

describe('popup session lifecycle', () => {
  const unlocked = { configured: true, vaultId: 'a', vaultName: 'A', locked: false, entriesVersion: 1 };
  it('rejects a delayed fetch after external lock even after unlocking again', () => {
    const guard = new SessionGuard(); guard.observe(unlocked, guard.probe()); const token = guard.capture();
    expect(guard.valid(token)).toBe(true);
    guard.observe({ ...unlocked, locked: true }, guard.probe()); expect(guard.valid(token)).toBe(false);
    guard.observe(unlocked, guard.probe()); expect(guard.valid(token)).toBe(false);
  });
  it('rejects old-vault and pre-edit responses', () => {
    const guard = new SessionGuard(); guard.observe(unlocked, guard.probe()); const token = guard.capture();
    guard.observe({ ...unlocked, vaultId: 'b' }, guard.probe()); expect(guard.valid(token)).toBe(false);
    const other = guard.capture(); guard.observe({ ...unlocked, vaultId: 'b', entriesVersion: 2 }, guard.probe()); expect(guard.valid(other)).toBe(false);
  });
  it('does not allow an older snapshot to undo a newer locked status', () => {
    const guard = new SessionGuard(); guard.observe(unlocked, guard.probe());
    const oldProbe = guard.probe(), newerProbe = guard.probe();
    guard.observe({ ...unlocked, locked: true }, newerProbe);
    expect(guard.observe(unlocked, oldProbe).accepted).toBe(false);
    expect(guard.valid(guard.capture())).toBe(false);
  });
  it('disconnect invalidates secrets and any earlier in-flight metadata', () => {
    const guard = new SessionGuard(); guard.observe(unlocked, guard.probe()); const token = guard.capture(), probe = guard.probe();
    guard.disconnect(); expect(guard.valid(token)).toBe(false); expect(guard.observe(unlocked, probe).accepted).toBe(false);
  });
});
