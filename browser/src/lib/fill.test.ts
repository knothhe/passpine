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
  it('can run from its source alone as an injected page function', () => {
    const doc = page('<form><input autocomplete="username"><input type="password"></form>');
    const injected = new Function('return (' + fillPage.toString() + ')')() as typeof fillPage;
    expect(injected(doc.URL, { username: 'alice', password: 'secret' })).toEqual({ filled: 2 });
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
  it('requires focus before writing to either of two login forms', () => {
    const doc = page('<form><input autocomplete="username"><input type="password"></form><form><input type="email"><input type="password"></form>');
    const changed = vi.fn();
    doc.addEventListener('input', changed);
    const credentials = { username: 'alice', password: 'secret' };
    expect(fillPage(doc.URL, credentials)).toEqual({ filled: 0, code: 'ambiguous_form' });
    expect(changed).not.toHaveBeenCalled();
    expect([...doc.querySelectorAll('input')].map(input => input.value)).toEqual(['', '', '', '']);
    doc.forms[1]!.querySelector('input')!.focus();
    expect(fillPage(doc.URL, credentials)).toEqual({ filled: 2 });
    expect([...doc.querySelectorAll('input')].map(input => input.value)).toEqual(['', '', 'alice', 'secret']);
  });

  it.each([
    ['<input form="step" autocomplete="section-login username webauthn">', 'alice'],
    ['<input form="step" type="password" autocomplete="section-login one-time-code">', '123456'],
  ])('honors a focused separate login step and its form owner', (html, expectedValue) => {
    const doc = page('<form id="other"><input type="email"><input type="password"></form><form id="step"></form>' + html);
    const target = doc.querySelector<HTMLInputElement>('[form="step"]')!;
    target.focus();
    expect(fillPage(doc.URL, { username: 'alice', password: 'secret', otp: '123456' })).toEqual({ filled: 1 });
    expect(target.value).toBe(expectedValue);
    expect([...doc.querySelectorAll<HTMLInputElement>('#other input')].every(input => !input.value)).toBe(true);
  });

  it.each(['', 'form', 'div role="form"'])('requires an explicit field for duplicate passwords in %s', wrapper => {
    const pair = '<input type="password"><input type="password">';
    const doc = page(wrapper ? `<${wrapper}>${pair}</${wrapper.split(' ')[0]}>` : pair);
    const [first, second] = doc.querySelectorAll('input');
    const credentials = { username: 'alice', password: 'secret' };
    expect(fillPage(doc.URL, credentials)).toEqual({ filled: 0, code: 'ambiguous_form' });
    second!.focus();
    expect(fillPage(doc.URL, credentials)).toEqual({ filled: 1 });
    expect(first!.value).toBe('');
    expect(second!.value).toBe('secret');
  });

  it.each([
    ['<input autocomplete="one-time-code">', { username: 'alice', password: 'secret' }],
    ['<input autocomplete="username">', { username: '', password: 'secret' }],
  ] as const)('considers only forms fillable by this entry and never falls back from an empty step', (step, credentials) => {
    const doc = page('<form><input type="password"></form><form id="step">' + step + '</form>');
    const password = doc.querySelector('input')!;
    expect(fillPage(doc.URL, credentials)).toEqual({ filled: 1 });
    expect(password.value).toBe('secret');
    password.value = '';
    doc.querySelector<HTMLInputElement>('#step input')!.focus();
    expect(fillPage(doc.URL, credentials)).toEqual({ filled: 0, code: 'no_fields' });
    expect(password.value).toBe('');
  });

  it('does not redirect a focused unowned new-password field to a different form', () => {
    const doc = page('<form><input type="password"></form><input autocomplete="new-password">');
    doc.querySelector<HTMLInputElement>('[autocomplete]')!.focus();
    expect(fillPage(doc.URL, { username: 'alice', password: 'secret' })).toEqual({ filled: 0, code: 'no_fields' });
    expect(doc.querySelector('input')!.value).toBe('');
  });

  it('leaves both forms untouched when focus is in a registration form', () => {
    const doc = page('<form><input autocomplete="username"><input type="password" autocomplete="section-signup new-password"></form><form><input type="email"><input type="password"></form>');
    doc.querySelector('input')!.focus();
    expect(fillPage(doc.URL, { username: 'alice', password: 'secret' })).toEqual({ filled: 0, code: 'no_fields' });
    expect([...doc.querySelectorAll('input')].every(input => !input.value)).toBe(true);
  });

  it('ignores hidden groups and avoids duplicating a password into an OTP control', () => {
    const doc = page('<div style="opacity:0"><form><input type="password"></form></div><form><input type="password" autocomplete="cc-csc"></form><form><input type="password" autocomplete="one-time-code"></form>');
    expect(fillPage(doc.URL, { username: 'alice', password: 'secret' })).toEqual({ filled: 0, code: 'no_fields' });
    expect(fillPage(doc.URL, { username: 'alice', password: 'secret', otp: '123456' })).toEqual({ filled: 1 });
    expect([...doc.querySelectorAll('input')].map(input => input.value)).toEqual(['', '', '123456']);
  });

  it('stops before the next write if the first input causes in-page navigation', () => {
    const doc = page('<form><input autocomplete="username"><input type="password"></form>');
    doc.querySelector('input')!.addEventListener('input', () => window.history.pushState({}, '', '/other'));
    expect(fillPage(doc.URL, { username: 'alice', password: 'secret' })).toEqual({ filled: 1, code: 'tab_changed' });
    expect(doc.querySelector<HTMLInputElement>('[type=password]')!.value).toBe('');
  });

  it('does not fill inside a nested frame', () => {
    const doc = page('<input autocomplete="username"><input type="password">');
    vi.stubGlobal('window', { top: {} });
    expect(fillPage('https://example.test/login', { username: 'alice', password: 'secret' })).toEqual({ filled: 0, code: 'tab_changed' });
    expect(doc.querySelector('input')!.value).toBe('');
  });
});
