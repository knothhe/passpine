import { useEffect, useRef, useState } from 'react';
import { Button, Callout, IconButton, TextArea, TextField } from '@radix-ui/themes';
import { Eye, EyeOff } from 'lucide-react';
import { parseDocument, updateDocument, validateEntry } from '../lib/model';
import { errorMessage } from '../lib/errors';
import type { Client } from '../lib/types';
import type { EditorValue } from './App';
import { PageHeader } from './Primitives';

export function EditorForm({ client, value, defaultURL, close, saved }: { client: Client; value: EditorValue; defaultURL: string; close(): void; saved(): Promise<void> }) {
  const [fields, setFields] = useState({ path: '', username: '', password: '', url: '', notes: '' });
  const [error, setError] = useState(''), [saving, setSaving] = useState(false), [reveal, setReveal] = useState(false);
  const session = useRef(0), savingRequest = useRef(false), defaultWebsite = useRef(defaultURL);
  defaultWebsite.current = defaultURL;
  useEffect(() => {
    session.current++;
    const doc = value.original ? parseDocument(value.original.contents) : undefined;
    setFields({ path: value.original?.entry.path ?? '', username: doc?.username ?? '', password: doc?.password ?? value.password ?? '', url: doc?.urls[0] ?? defaultWebsite.current, notes: doc?.notes ?? '' });
    setError(''); setReveal(false);
    return () => { session.current++; };
  }, [value]);
  const update = (key: keyof typeof fields, next: string) => setFields(previous => ({ ...previous, [key]: next }));
  const save = async (event: React.FormEvent) => {
    event.preventDefault();
    if (savingRequest.current) return;
    if (!validateEntry(fields.path)) { setError('请输入有效名称，可用「文件夹/账号」分类；不能含隐藏路径或特殊分隔符。'); return; }
    if (!fields.password || /[\r\n]/.test(fields.password)) { setError('密码不能为空，也不能包含换行。'); return; }
    if (/[\r\n]/.test(fields.username + fields.url)) { setError('用户名和网站不能包含换行。'); return; }
    const current = session.current;
    savingRequest.current = true;
    setSaving(true); setError('');
    try {
      await client.request({ action: 'save', vaultId: value.vaultId, entry: value.original?.entry.path ?? fields.path, destination: value.original && value.original.entry.path !== fields.path ? fields.path : undefined,
        mode: value.original ? 'edit' : 'create', revision: value.original?.revision, contents: updateDocument(value.original?.contents ?? '', fields) });
      if (current === session.current) await saved();
    } catch (error) { if (current === session.current) setError(errorMessage(error)); }
    finally { savingRequest.current = false; if (current === session.current) setSaving(false); }
  };
  return <form className="popup-screen editor-screen" onSubmit={event => void save(event)}>
    <PageHeader title={value.original ? '编辑账号' : '新建账号'} backLabel={value.original ? '返回账号详情' : '返回密码库'} back={close} disabled={saving}/>
    <main className="screen-content editor-content">
      <fieldset className="editor-fields" disabled={saving}>
        <label className="field-label">名称<TextField.Root autoFocus required value={fields.path} placeholder="github.com/个人账号" onChange={event => update('path', event.target.value)}/></label>
        <label className="field-label">用户名<TextField.Root value={fields.username} autoComplete="off" placeholder="用户名或邮箱" onChange={event => update('username', event.target.value)}/></label>
        <label className="field-label">密码<TextField.Root required type={reveal ? 'text' : 'password'} value={fields.password} autoComplete="new-password" onChange={event => update('password', event.target.value)}><TextField.Slot side="right"><IconButton type="button" aria-label="显示或隐藏密码" variant="ghost" size="1" onClick={() => setReveal(!reveal)}>{reveal ? <EyeOff size={15}/> : <Eye size={15}/>}</IconButton></TextField.Slot></TextField.Root></label>
        <label className="field-label">网站<TextField.Root value={fields.url} autoComplete="url" placeholder="https://example.com" onChange={event => update('url', event.target.value)}/></label>
        <label className="field-label">备注<TextArea value={fields.notes} placeholder="添加备注…" rows={3} onChange={event => update('notes', event.target.value)}/></label>
      </fieldset>
    </main>
    {error && <Callout.Root color="red" size="1" role="alert" className="screen-error"><Callout.Text>{error}</Callout.Text></Callout.Root>}
    <footer className="screen-footer"><Button type="button" color="gray" variant="soft" disabled={saving} onClick={close}>取消</Button><Button type="submit" className="grow" loading={saving}>保存</Button></footer>
  </form>;
}
