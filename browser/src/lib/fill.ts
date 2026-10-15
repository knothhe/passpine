// Serialized by scripting.executeScript. Keep this function self-contained.
export function fillPage(expectedURL: string, credentials: { username: string; password: string; otp?: string }): { filled: number; code?: string } {
  if (window !== window.top || location.href !== expectedURL || !['http:', 'https:'].includes(location.protocol)) return { filled: 0, code: 'tab_changed' };
  const visible = (element: HTMLInputElement) => {
    const style = getComputedStyle(element), rect = element.getBoundingClientRect();
    return !element.disabled && !element.readOnly && element.type !== 'hidden' && style.display !== 'none' && style.visibility !== 'hidden' && style.opacity !== '0' && rect.width > 2 && rect.height > 2;
  };
  const inputs = Array.from(document.querySelectorAll<HTMLInputElement>('input')).filter(visible);
  const focused = document.activeElement instanceof HTMLInputElement && inputs.includes(document.activeElement) ? document.activeElement : undefined;
  const passwords = inputs.filter(input => input.type === 'password' && input.autocomplete !== 'new-password');
  const password = focused && passwords.includes(focused) ? focused : passwords.find(input => focused?.form && input.form === focused.form) ?? passwords[0];
  const scope = password?.form ?? focused?.form;
  const candidates = scope ? inputs.filter(input => input.form === scope) : inputs;
  const username = candidates.find(input => input.autocomplete === 'username') ?? candidates.find(input => input.type === 'email') ?? candidates.find(input => ['text', 'tel'].includes(input.type) && /user|login|email|account/i.test(`${input.name} ${input.id} ${input.autocomplete}`));
  const otp = candidates.find(input => input.autocomplete === 'one-time-code');
  const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')?.set;
  if (!setter) return { filled: 0, code: 'no_fields' };
  let filled = 0;
  for (const [input, value] of [[username, credentials.username], [password, credentials.password], [otp, credentials.otp]] as const) {
    if (!input || !value) continue;
    if (location.href !== expectedURL) return { filled, code: 'tab_changed' };
    setter.call(input, value);
    input.dispatchEvent(new Event('input', { bubbles: true }));
    input.dispatchEvent(new Event('change', { bubbles: true }));
    filled++;
  }
  return { filled, ...(filled ? {} : { code: 'no_fields' }) };
}
