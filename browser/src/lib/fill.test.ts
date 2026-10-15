import { afterEach, describe, expect, it, vi } from 'vitest';
import { JSDOM } from 'jsdom';
import { fillPage } from './fill';

function page(html: string, url = 'https://example.test/login') {
  const dom = new JSDOM(html, { url });
  const window = dom.window;
  for (const [key, value] of Object.entries({ window, document: window.document, location: window.location, HTMLInputElement: window.HTMLInputElement, Event: window.Event, getComputedStyle: window.getComputedStyle.bind(window) })) vi.stubGlobal(key, value);
  vi.spyOn(window.HTMLInputElement.prototype, 'getBoundingClientRect').mockImplementation(() => ({ width: 200, height: 30, x: 0, y: 0, top: 0, left: 0, right: 200, bottom: 30, toJSON() {} }));
  return window.document;
}
afterEach(() => { vi.restoreAllMocks(); vi.unstubAllGlobals(); });
describe('explicit top-frame form filling', () => {
  it('fills visible login fields using native setters and input/change events, without submitting', () => {
    const doc = page('<form><input autocomplete="username"><input type="password"><button>Sign in</button></form>');
    let inputs = 0, changes = 0, submits = 0;
    doc.addEventListener('input', () => inputs++); doc.addEventListener('change', () => changes++); doc.addEventListener('submit', () => submits++);
    expect(fillPage('https://example.test/login', { username: 'alice', password: 'secret' })).toEqual({ filled: 2 });
    expect(doc.querySelector('input')!.value).toBe('alice'); expect(doc.querySelector<HTMLInputElement>('input[type=password]')!.value).toBe('secret');
    expect({ inputs, changes, submits }).toEqual({ inputs: 2, changes: 2, submits: 0 });
  });
  it('refuses a stale URL before touching any fields', () => {
    const doc = page('<input autocomplete="username"><input type="password">', 'https://evil.test/login');
    expect(fillPage('https://example.test/login', { username: 'alice', password: 'secret' })).toEqual({ filled: 0, code: 'tab_changed' });
    expect([...doc.querySelectorAll('input')].every(input => input.value === '')).toBe(true);
  });
  it('ignores hidden, disabled and new-password fields', () => {
    const doc = page('<input autocomplete="username" style="display:none"><input type="password" disabled><input type="password" autocomplete="new-password">');
    expect(fillPage('https://example.test/login', { username: 'alice', password: 'secret' })).toEqual({ filled: 0, code: 'no_fields' });
    expect([...doc.querySelectorAll('input')].every(input => input.value === '')).toBe(true);
  });
  it('does not fill a focused new-password field', () => {
    const doc = page('<form><input type="password" autocomplete="new-password"></form>');
    doc.querySelector('input')!.focus();
    expect(fillPage('https://example.test/login', { username: 'alice', password: 'secret' })).toEqual({ filled: 0, code: 'no_fields' });
    expect(doc.querySelector('input')!.value).toBe('');
  });
  it('keeps username and password in the selected form', () => {
    const doc = page('<form id="signup"><input type="email"><input type="password" autocomplete="new-password"></form><form id="login"><input autocomplete="username"><input type="password"></form>');
    fillPage('https://example.test/login', { username: 'alice', password: 'secret' });
    expect(doc.querySelector<HTMLInputElement>('#signup input')!.value).toBe('');
    expect(doc.querySelector<HTMLInputElement>('#login input')!.value).toBe('alice');
  });
  it('does not fill inside a nested frame', () => {
    const doc = page('<input autocomplete="username"><input type="password">');
    vi.stubGlobal('window', { top: {} });
    expect(fillPage('https://example.test/login', { username: 'alice', password: 'secret' })).toEqual({ filled: 0, code: 'tab_changed' });
    expect(doc.querySelector('input')!.value).toBe('');
  });
});
