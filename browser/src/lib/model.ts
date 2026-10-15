import type { Credentials, Entry, Snapshot, TabContext } from './types';

export function website(raw: string): URL | undefined {
  try {
    const url = new URL(raw.includes('://') ? raw : `https://${raw}`);
    if (!['https:', 'http:'].includes(url.protocol) || url.username || url.password) return undefined;
    return url;
  } catch { return undefined; }
}

export function tabContext(tab: { id?: number; url?: string }): TabContext | undefined {
  if (tab.id === undefined || !tab.url || !/^https?:\/\//.test(tab.url)) return undefined;
  const url = website(tab.url);
  return url ? { id: tab.id, url: tab.url, hostname: url.hostname, secure: url.protocol === 'https:' } : undefined;
}

// An exact host match is deliberate: siblings and lookalike domains never inherit trust.
export function pathMatchesHost(path: string, hostname: string): boolean {
  return path.split('/').some(part => {
    if (!part.includes('.') || part.includes('@')) return false;
    return website(part)?.hostname === hostname;
  });
}

export function credentialMatchesHost(document: Credentials, hostname: string): boolean {
  return document.urls.some(value => website(value)?.hostname === hostname);
}

export function parseDocument(contents: string): Credentials {
  const lines = contents.replace(/\r\n/g, '\n').split('\n');
  const password = lines.shift() ?? '';
  const fields: Credentials['fields'] = [];
  const notes: string[] = [];
  for (const line of lines) {
    const match = /^([\p{L}\p{N}_-]+):\s?(.*)$/u.exec(line);
    if (match) {
      const key = match[1]!, isNote = ['notes', 'note'].includes(key.toLowerCase());
      const value = isNote ? line.slice(key.length + 1).replace(/^ /, '') : match[2]!;
      fields.push({ key, value });
      if (isNote) notes.push(value);
    }
    else notes.push(line);
  }
  const value = (keys: string[]) => fields.find(field => keys.includes(field.key.toLowerCase()))?.value ?? '';
  return {
    password, username: value(['login', 'username', 'user', 'email']),
    urls: fields.filter(field => ['url', 'uri', 'website', 'site', 'link'].includes(field.key.toLowerCase())).map(field => field.value),
    otp: value(['otp', 'totp']), notes: notes.join('\n').trim(), fields,
  };
}

export function updateDocument(original: string, values: { password: string; username: string; url: string; notes: string }): string {
  const document = parseDocument(original);
  if (values.password === document.password && values.username === document.username && values.url === (document.urls[0] ?? '') && values.notes === document.notes) return original;

  // Keep each line's original terminator and every untouched field verbatim.
  // The structured editor changes the first displayed alias, never all aliases.
  type Line = { text: string; ending: string };
  let lines: Line[] = Array.from(original.matchAll(/([^\r\n]*)(\r\n|\n|\r|$)/g)).filter(match => match[0] !== '').map(match => ({ text: match[1]!, ending: match[2]! }));
  if (!lines.length) lines = [{ text: '', ending: '' }];
  const newline = lines.find(line => line.ending)?.ending ?? '\n';
  const field = (line: Line) => /^([\p{L}\p{N}_-]+):\s?(.*)$/u.exec(line.text);
  const noteLine = (line: Line) => { const match = field(line); return !match || ['notes', 'note'].includes(match[1]!.toLowerCase()); };
  const changeField = (aliases: string[], key: string, value: string) => {
    const line = lines.slice(1).find(candidate => { const match = field(candidate); return !!match && aliases.includes(match[1]!.toLowerCase()); });
    if (line) { const match = field(line)!; line.text = line.text.slice(0, line.text.length - match[2]!.length) + value; }
    else if (value) { if (!lines[0]!.ending) lines[0]!.ending = newline; lines.splice(1, 0, { text: `${key}: ${value}`, ending: newline }); }
  };
  if (values.password !== document.password) lines[0]!.text = values.password;
  if (values.username !== document.username) changeField(['login', 'username', 'user', 'email'], 'username', values.username);
  if (values.url !== (document.urls[0] ?? '')) changeField(['url', 'uri', 'website', 'site', 'link'], 'url', values.url);
  if (values.notes !== document.notes) {
    const firstNote = lines.findIndex((line, index) => index > 0 && noteLine(line) && line.text.trim() !== '');
    const position = firstNote < 0 ? lines.length : firstNote;
    const insertAt = lines.slice(0, position).filter((line, index) => index === 0 || !noteLine(line)).length;
    lines = lines.filter((line, index) => index === 0 || !noteLine(line));
    if (values.notes) {
      // A notes field per line preserves literal colons and blank lines in both clients.
      const notes = values.notes.split(/\r?\n/).map(text => ({ text: `notes: ${text}`, ending: newline }));
      if (insertAt === lines.length && !/[\r\n]$/.test(original)) notes.at(-1)!.ending = '';
      if (insertAt > 0 && !lines[insertAt - 1]!.ending) lines[insertAt - 1]!.ending = newline;
      lines.splice(insertAt, 0, ...notes);
    }
  }
  return lines.map(line => line.text + line.ending).join('');
}

export function validateEntry(path: string): boolean {
  return path.length > 0 && path.length <= 255 && !/[\x00-\x1f\x7f\\:]/.test(path) &&
    path.split('/').every(part => part.length > 0 && !/^[.-]/.test(part) && !/[. ]$/.test(part));
}

export function entriesFor(snapshot: Snapshot, query: string, scope: 'all' | 'site' = 'all'): Entry[] {
  const terms = query.trim().toLocaleLowerCase().split(/\s+/).filter(Boolean);
  return snapshot.entries.map(({ path }) => {
    const parts = path.split('/');
    const title = parts.pop() ?? path;
    return { path, title, folder: parts.join('/'), matched: snapshot.matches.includes(path) };
  }).filter(entry => (scope === 'all' || entry.matched) && terms.every(term => entry.path.toLocaleLowerCase().includes(term)))
    .sort((a, b) => Number(b.matched) - Number(a.matched) || a.path.localeCompare(b.path));
}

export function sameTab(expected: TabContext, actual: { id?: number; url?: string }): boolean {
  return expected.id === actual.id && expected.url === actual.url && Boolean(tabContext(actual));
}

export async function associationKey(vaultId: string, entry: string, hostname: string): Promise<string> {
  const bytes = new TextEncoder().encode(JSON.stringify([vaultId, entry, hostname]));
  const digest = await crypto.subtle.digest('SHA-256', bytes);
  return Array.from(new Uint8Array(digest), n => n.toString(16).padStart(2, '0')).join('');
}
