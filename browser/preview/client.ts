// This module is imported only by the standalone preview, never by extension entrypoints.
import { PassPineError } from '../src/lib/errors';
import type { Client, EntryDocument, Snapshot, UIRequest } from '../src/lib/types';

const examples: Record<string, string> = {
  'github.com/个人账号': 'preview-only-password\nusername: alex@hey.com\nurl: https://github.com\n\n个人项目与开源代码。',
  'github.com/工作账号': 'preview-only-password\nusername: alex@studio.design\nurl: https://github.com\n\n团队工作空间。',
  'linear.app/Studio': 'preview-only-password\nusername: alex@studio.design\nurl: https://linear.app',
  'notion.so/我的工作空间': 'preview-only-password\nusername: alex@hey.com\nurl: https://notion.so',
  'vercel.com/个人项目': 'preview-only-password\nusername: alex@hey.com\nurl: https://vercel.com',
  'figma.com/Design team': 'preview-only-password\nusername: alex@studio.design\nurl: https://figma.com',
};
let locked = false;
let revision = 1;
const matches = new Set(Object.keys(examples).filter(path => path.startsWith('github.com/')));
export function setPreviewState(state: 'connected' | 'locked' | 'disconnected') { locked = state === 'locked'; disconnected = state === 'disconnected'; }
let disconnected = false;
const snapshot = (): Snapshot => ({ configured: true, vaultId: 'demo-vault', vaultName: '个人密码库', locked, entriesVersion: revision, capabilities: ['fetch', 'save', 'delete', 'copy', 'generate'], entries: Object.keys(examples).map(path => ({ path, name: path.split('/').at(-1)!, folder: path.split('/').slice(0, -1).join('/'), favorite: false })), tab: { id: 1, url: 'https://github.com/login', hostname: 'github.com', secure: true }, matches: [...matches] });

export const demoClient: Client = {
  demo: true,
  async request<T>(request: UIRequest): Promise<T> {
    if (disconnected) throw new PassPineError('app_unavailable');
    let result: unknown = {};
    if (request.action === 'snapshot' || request.action === 'status') result = snapshot();
    else if (request.action === 'lock') locked = true;
    else if (request.action === 'unlock') locked = false;
    else if (request.action === 'generate') {
      const alphabets = [request.options.lowercase ? 'abcdefghijkmnpqrstuvwxyz' : '', request.options.uppercase ? 'ABCDEFGHJKLMNPQRSTUVWXYZ' : '', request.options.numbers ? '23456789' : '', request.options.symbols ? '!@#$%&*_-+=' : ''].filter(Boolean);
      const pool = alphabets.join('');
      if (!pool) throw new Error('请至少选择一种字符类型。');
      const bytes = crypto.getRandomValues(new Uint8Array(request.options.length));
      result = { password: Array.from(bytes, byte => pool[byte % pool.length]).join('') };
    } else {
      if (locked) throw new PassPineError('locked');
      if (request.action === 'fetch') result = { contents: examples[request.entry] ?? '', revision: String(revision) } satisfies EntryDocument;
      else if (request.action === 'save') { if (request.mode === 'create' && examples[request.entry]) throw new Error('此名称已存在。'); if (request.destination) delete examples[request.entry]; examples[request.destination ?? request.entry] = request.contents; revision++; }
      else if (request.action === 'delete') { delete examples[request.entry]; revision++; }
      else if (request.action === 'associate') matches.add(request.entry);
      else if (request.action === 'fill') result = { filled: 2 };
    }
    return result as T;
  },
  async openOptions() { window.open('/?view=options', '_blank', 'noopener'); },
  async copy(text: string) { await navigator.clipboard.writeText(text); },
};
