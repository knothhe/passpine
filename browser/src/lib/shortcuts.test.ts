import { describe, expect, it, vi } from 'vitest';
import { assignedShortcuts, captureShortcut, defaultShortcuts, FILL_COMMAND, OPEN_COMMAND, saveShortcut, type ShortcutAPI } from './shortcuts';

const keyEvent = (values: Partial<Parameters<typeof captureShortcut>[0]> = {}): Parameters<typeof captureShortcut>[0] => ({ key: 'K', code: 'KeyK', ctrlKey: false, altKey: false, metaKey: false, shiftKey: false, isComposing: false, ...values });

describe('extension shortcuts', () => {
  it('retains legacy default combinations without inventing bindings when a command is unassigned', () => {
    expect(defaultShortcuts(false)).toEqual({ [OPEN_COMMAND]: 'Ctrl+Shift+L', [FILL_COMMAND]: 'Ctrl+Shift+F' });
    expect(defaultShortcuts(true)).toEqual({ [OPEN_COMMAND]: 'Command+Shift+L', [FILL_COMMAND]: 'Command+Shift+F' });
    expect(assignedShortcuts([{ name: OPEN_COMMAND, shortcut: 'Alt+K' }, { name: FILL_COMMAND, shortcut: '' }])).toEqual({ [OPEN_COMMAND]: 'Alt+K', [FILL_COMMAND]: '' });
    expect(assignedShortcuts([])[OPEN_COMMAND]).toBe('');
  });

  it('records physical letter keys and distinguishes macOS Control from Command', () => {
    expect(captureShortcut(keyEvent({ key: '˚', altKey: true, metaKey: true }), true)).toEqual({ type: 'shortcut', shortcut: 'Alt+Command+K' });
    expect(captureShortcut(keyEvent({ ctrlKey: true, shiftKey: true }), true)).toEqual({ type: 'shortcut', shortcut: 'MacCtrl+Shift+K' });
    expect(captureShortcut(keyEvent({ ctrlKey: true, shiftKey: true }), false)).toEqual({ type: 'shortcut', shortcut: 'Ctrl+Shift+K' });
  });

  it('records supported punctuation when modifiers change the printed character', () => {
    expect(captureShortcut(keyEvent({ key: '<', code: 'Comma', ctrlKey: true, shiftKey: true }), false)).toEqual({ type: 'shortcut', shortcut: 'Ctrl+Shift+Comma' });
    expect(captureShortcut(keyEvent({ key: '≥', code: 'Period', altKey: true }), true)).toEqual({ type: 'shortcut', shortcut: 'Alt+Period' });
  });

  it('rejects unsupported combinations and supports cancelling and clearing without consuming Tab', () => {
    expect(captureShortcut(keyEvent(), false).type).toBe('invalid');
    expect(captureShortcut(keyEvent({ ctrlKey: true, shiftKey: true, altKey: true }), false).type).toBe('invalid');
    expect(captureShortcut(keyEvent({ metaKey: true }), false).type).toBe('invalid');
    expect(captureShortcut(keyEvent({ key: 'Control' }), false).type).toBe('pending');
    expect(captureShortcut(keyEvent({ isComposing: true }), false).type).toBe('pending');
    expect(captureShortcut(keyEvent({ key: 'Escape' }), false).type).toBe('cancel');
    expect(captureShortcut(keyEvent({ key: 'Backspace' }), false)).toEqual({ type: 'shortcut', shortcut: '' });
  });

  it('saves through the browser API and rereads the actual accepted assignment', async () => {
    const getAll = vi.fn().mockResolvedValueOnce([{ name: FILL_COMMAND, shortcut: 'Ctrl+Shift+F' }]).mockResolvedValueOnce([{ name: OPEN_COMMAND, shortcut: 'Alt+K' }, { name: FILL_COMMAND, shortcut: 'Ctrl+Shift+F' }]);
    const update = vi.fn().mockResolvedValue(undefined);
    const result = await saveShortcut({ getAll, update }, OPEN_COMMAND, 'Alt+K');
    expect(update).toHaveBeenCalledExactlyOnceWith({ name: OPEN_COMMAND, shortcut: 'Alt+K' });
    expect(result[OPEN_COMMAND]).toBe('Alt+K');
    expect(getAll).toHaveBeenCalledTimes(2);
  });

  it('prevents duplicate assignments before changing browser configuration', async () => {
    const api: ShortcutAPI = { getAll: vi.fn().mockResolvedValue([{ name: FILL_COMMAND, shortcut: '⌘⇧F' }]), update: vi.fn() };
    await expect(saveShortcut(api, OPEN_COMMAND, 'Command+Shift+F')).rejects.toThrow('此快捷键已用于另一项操作');
    expect(api.update).not.toHaveBeenCalled();
  });

  it('keeps failed updates from being presented as saved and permits explicitly clearing a command', async () => {
    const api: ShortcutAPI = { getAll: vi.fn().mockResolvedValue([]), update: vi.fn().mockRejectedValue(new Error('browser rejected')) };
    await expect(saveShortcut(api, FILL_COMMAND, 'Alt+K')).rejects.toThrow('browser rejected');
    expect(api.getAll).toHaveBeenCalledTimes(1);
    api.update = vi.fn().mockResolvedValue(undefined);
    expect((await saveShortcut(api, FILL_COMMAND, ''))[FILL_COMMAND]).toBe('');
    expect(api.update).toHaveBeenCalledWith({ name: FILL_COMMAND, shortcut: '' });
  });


});
