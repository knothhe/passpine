import { describe, expect, it } from 'vitest';
import { associationKey, credentialMatchesHost, entriesFor, parseDocument, pathMatchesHost, sameTab, tabContext, updateDocument, validateEntry, website } from './model';
import type { Snapshot } from './types';

describe('site boundary', () => {
  it('accepts exact host components and refuses lookalikes, siblings, suffixes, email addresses', () => {
    expect(pathMatchesHost('work/github.com/alex', 'github.com')).toBe(true);
    for (const path of ['github.com.evil.test/alex', 'evilgithub.com/alex', 'alex@github.com', 'login.github.com/alex', 'github.com/alex']) {
      expect(pathMatchesHost(path, 'evil.test')).toBe(false);
    }
    expect(pathMatchesHost('github.com/alex', 'login.github.com')).toBe(false);
    expect(pathMatchesHost('github.com.evil.test/alex', 'github.com')).toBe(false);
    expect(credentialMatchesHost(parseDocument('secret\nurl: https://github.com/login'), 'github.com')).toBe(true);
    expect(credentialMatchesHost(parseDocument('secret\nurl: https://github.com.evil.test'), 'github.com')).toBe(false);
    expect(credentialMatchesHost(parseDocument('secret\nurl: https://github.com@evil.test'), 'evil.test')).toBe(false);
  });
  it('rejects internal browser pages and URL/frame changes', () => {
    expect(tabContext({ id: 1, url: 'chrome://settings' })).toBeUndefined();
    expect(tabContext({ id: 1, url: 'javascript:alert(1)' })).toBeUndefined();
    expect(website('file:///etc/passwd')).toBeUndefined();
    const tab = tabContext({ id: 1, url: 'https://github.com/login' })!;
    expect(sameTab(tab, { id: 1, url: tab.url })).toBe(true);
    expect(sameTab(tab, { id: 2, url: tab.url })).toBe(false);
    expect(sameTab(tab, { id: 1, url: 'https://github.com/logout' })).toBe(false);
  });
  it('hashes vault, entry, and host together without plaintext storage keys', async () => {
    const one = await associationKey('vault-a', 'github.com/alex', 'github.com');
    expect(one).toMatch(/^[a-f0-9]{64}$/);
    expect(one).not.toEqual(await associationKey('vault-b', 'github.com/alex', 'github.com'));
    expect(one).not.toEqual(await associationKey('vault-a', 'github.com/alex', 'evil.test'));
  });
});

describe('password document', () => {
  it.each(['notes', 'note'])('reads a native single-line %s field', key => {
    expect(parseDocument(`secret\nusername: alice\n${key}: native note\n`).notes).toBe('native note');
  });
  it('combines multiline, repeated note aliases and body notes in their original order', () => {
    const raw = 'secret\r\nbody before\r\nnotes: first\r\ncontinued\r\ncustom: retained\r\n\r\nNOTE:\tsecond\r\nbody after\r\nnotes: third\r\n';
    expect(parseDocument(raw).notes).toBe('body before\nfirst\ncontinued\n\n\tsecond\nbody after\nthird');
    expect(parseDocument(raw).fields).toContainEqual({ key: 'custom', value: 'retained' });
  });
  it('preserves untouched note fields and body text verbatim across rename and other edits', () => {
    const original = 'secret\r\nlogin: alice\r\nnotes:  first  \nbody line\r\nNOTE: second\r\n\r\n';
    const document = parseDocument(original);
    const values = { password: document.password, username: document.username, url: '', notes: document.notes };
    expect(updateDocument(original, values)).toBe(original);
    expect(updateDocument(original, { ...values, username: 'bob' })).toBe(original.replace('login: alice', 'login: bob'));
  });
  it('replaces or clears both old note fields and body notes without duplicating them', () => {
    const original = 'secret\nlogin: alice\nnotes: old field\nold body\ncustom: retain\nNOTE: second old field\n';
    const document = parseDocument(original);
    const values = { password: document.password, username: document.username, url: '', notes: 'label: literal note\nnext line\n\nlast line' };
    const replaced = updateDocument(original, values);
    expect(parseDocument(replaced).notes).toBe(values.notes);
    expect(replaced).not.toContain('old');
    expect(replaced).toContain('custom: retain\n');
    expect(replaced.indexOf('notes: label: literal note')).toBeLessThan(replaced.indexOf('custom: retain'));
    expect(updateDocument(original, { ...values, notes: '' })).toBe('secret\nlogin: alice\ncustom: retain\n');
  });
  it('keeps the document byte-for-byte when only the entry path changes', () => {
    const original = 'secret\r\nlogin: alice\r\nusername: alternate\ncustom: keep:colon\r\n\r\n  first note  \r\n\nlast note\r\n';
    const doc = parseDocument(original);
    expect(updateDocument(original, { password: doc.password, username: doc.username, url: doc.urls[0] ?? '', notes: doc.notes })).toBe(original);
  });
  it('changes only the displayed field and preserves aliases, whitespace and line endings', () => {
    const original = 'secret\r\nlogin:\talice\r\nusername: alternate\nurl: https://example.com\r\n\r\n  note with spaces  \r\n';
    const doc = parseDocument(original);
    expect(updateDocument(original, { password: doc.password, username: 'bob', url: doc.urls[0]!, notes: doc.notes })).toBe(original.replace('login:\talice', 'login:\tbob'));
    expect(updateDocument(original, { password: 'new', username: doc.username, url: doc.urls[0]!, notes: doc.notes })).toBe(original.replace('secret', 'new'));
  });
  it('preserves custom fields, OTP and secondary URLs during structured edits', () => {
    const original = 'old\r\nlogin: alice\r\nurl: https://example.com\r\nurl: https://accounts.example.com\r\ntotp: otpauth://totp/example?secret=DEMO\r\ncustom: keep:colon\r\n\r\nprivate note';
    const next = updateDocument(original, { password: 'new', username: 'bob', url: 'https://new.test', notes: 'changed note' });
    expect(parseDocument(next)).toMatchObject({ password: 'new', username: 'bob', urls: ['https://new.test', 'https://accounts.example.com'], otp: 'otpauth://totp/example?secret=DEMO', notes: 'changed note' });
    expect(next).toContain('custom: keep:colon');
  });
  it('rejects traversal, hidden entries and malformed entry names', () => {
    for (const value of ['../secret', '/absolute', 'a//b', 'a/..', 'a/.hidden', '-option', 'foo\\bar', 'foo\nbar', 'foo:bar']) expect(validateEntry(value)).toBe(false);
    expect(validateEntry('github.com/个人账号')).toBe(true);
  });
  it('searches the full vault while keeping site matches first', () => {
    const snapshot = { entries: [{ path: 'z.test/personal' }, { path: 'a.test/work' }, { path: 'github.com/work' }], matches: ['github.com/work'] } as Snapshot;
    expect(entriesFor(snapshot, 'work').map(entry => entry.path)).toEqual(['github.com/work', 'a.test/work']);
    expect(entriesFor(snapshot, '', 'site').map(entry => entry.path)).toEqual(['github.com/work']);
  });
});
