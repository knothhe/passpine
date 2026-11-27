// Serialized by scripting.executeScript. Keep this function self-contained.
export function fillPage(expectedURL: string, credentials: { username: string; password: string; otp?: string }): { filled: number; code?: string } {
  if (window !== window.top || location.href !== expectedURL || !['http:', 'https:'].includes(location.protocol)) return { filled: 0, code: 'tab_changed' };

  const visible = (input: HTMLInputElement) => {
    if (input.matches(':disabled') || input.readOnly || input.type === 'hidden' || input.closest('[hidden], [inert]')) return false;
    const rect = input.getBoundingClientRect();
    if (rect.width <= 2 || rect.height <= 2 || getComputedStyle(input).visibility !== 'visible') return false;
    for (let element: Element | null = input; element; element = element.parentElement) {
      const style = getComputedStyle(element);
      if (style.opacity === '0' || style.display === 'none') return false;
    }
    return true;
  };
  const owner = (input: HTMLInputElement) => input.form ?? input.closest('[role="form"]');
  const tokens = (input: HTMLInputElement) => (input.getAttribute('autocomplete') ?? '').toLowerCase().split(/\s+/);
  type Field = 'username' | 'password' | 'otp';
  const field = (input: HTMLInputElement): Field | 'new-password' | undefined => {
    const hints = tokens(input);
    if (hints.includes('new-password')) return 'new-password';
    if (hints.includes('one-time-code') && ['text', 'tel', 'password', 'number'].includes(input.type)) return 'otp';
    if (input.type === 'password') {
      if (hints.includes('username')) return 'username';
      if (hints.includes('current-password') || hints.every(hint => !hint || hint === 'on' || hint === 'off')) return 'password';
      return;
    }
    if (!['text', 'tel', 'email'].includes(input.type)) return;
    if (hints.includes('username') || hints.includes('email')) return 'username';
    if (hints.some(hint => hint && hint !== 'on' && hint !== 'off')) return;
    if (input.type === 'email' || /user|login|email|account/i.test(`${input.name} ${input.id}`)) return 'username';
  };

  type LoginGroup = { scope: Element | null; newPassword: boolean; fields: { input: HTMLInputElement; field: Field }[] };
  const focused = document.activeElement;
  const focusedInput = focused instanceof HTMLInputElement ? focused : undefined;
  const focusedRole = focusedInput && field(focusedInput);
  if (focusedRole === 'new-password') return { filled: 0, code: 'no_fields' };
  const focusedOwner = focusedInput ? owner(focusedInput) : focused?.closest('form, [role="form"]');
  const useFocus = !!focusedOwner || !!focusedRole;

  // Use the form owner, including controls outside <form> linked via form="id".
  // Unowned controls form a separate group. Never fall back from a focused form.
  const groups = new Map<Element | null, LoginGroup>();
  for (const input of document.querySelectorAll('input')) {
    const role = field(input);
    if (!role || (role !== 'new-password' && !credentials[role])) continue;
    const scope = owner(input);
    if ((useFocus && scope !== focusedOwner) || !visible(input)) continue;
    let group = groups.get(scope);
    if (!group) {
      group = { scope, newPassword: false, fields: [] };
      groups.set(scope, group);
    }
    if (role === 'new-password') group.newPassword = true;
    else group.fields.push({ input, field: role });
  }
  // Registration and password-change groups can also contain an ordinary username.
  const candidates = [...groups.values()].filter(group => !group.newPassword && group.fields.length);
  const target = candidates[0];
  if (!target) return { filled: 0, code: 'no_fields' };
  if (candidates.length > 1) return { filled: 0, code: 'ambiguous_form' };
  let fields = target.fields;
  if (new Set(fields.map(item => item.field)).size !== fields.length) {
    // Without distinct form owners, duplicate roles cannot safely be paired.
    // The user's focused field can still be filled explicitly on its own.
    fields = fields.filter(item => item.input === focusedInput);
    if (!fields.length) return { filled: 0, code: 'ambiguous_form' };
  }
  const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')?.set;
  if (!setter) return { filled: 0, code: 'no_fields' };
  let filled = 0;
  for (const role of ['username', 'password', 'otp'] as const) {
    const input = fields.find(item => item.field === role)?.input;
    if (!input) continue;
    if (location.href !== expectedURL || !input.isConnected || owner(input) !== target.scope || !visible(input) || field(input) !== role) {
      return { filled, code: 'tab_changed' };
    }
    setter.call(input, credentials[role]);
    input.dispatchEvent(new Event('input', { bubbles: true }));
    input.dispatchEvent(new Event('change', { bubbles: true }));
    filled++;
  }
  if (location.href !== expectedURL) return { filled, code: 'tab_changed' };
  return filled ? { filled } : { filled: 0, code: 'no_fields' };
}
