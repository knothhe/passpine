import { useEffect, useRef, useState } from 'react';
import { Button, Callout, Dialog, Flex, IconButton, TextArea, TextField } from '@radix-ui/themes';
import { Eye, EyeOff, Shuffle } from 'lucide-react';
import { parseDocument, updateDocument, validateEntry } from '../lib/model';
import { errorMessage } from '../lib/errors';
import type { Client } from '../lib/types';
import type { EditorValue } from './App';
import { initialGenerator } from './Generator';

export function EditorForm({ client, value, defaultURL, close, saved }: { client: Client; value?: EditorValue; defaultURL: string; close(): void; saved(): Promise<void> }) {
  const [fields, setFields] = useState({ path: '', username: '', password: '', url: '', notes: '' });
  const [error, setError] = useState(''), [saving, setSaving] = useState(false), [reveal, setReveal] = useState(false);
  const session = useRef(0);
  useEffect(() => {
    session.current++;
    if (!value) { setFields({ path: '', username: '', password: '', url: '', notes: '' }); return; }
    const doc = value.original ? parseDocument(value.original.contents) : undefined;
    setFields({ path: value.original?.entry.path ?? '', username: doc?.username ?? '', password: doc?.password ?? value.password ?? '', url: doc?.urls[0] ?? defaultURL, notes: doc?.notes ?? '' });
    setError(''); setReveal(false);
  }, [value, defaultURL]);
  const update = (key: keyof typeof fields, next: string) => setFields(previous => ({ ...previous, [key]: next }));
  const generate = async () => {
    const current = session.current;
    try { const data = await client.request<{ password: string }>({ action: 'generate', options: initialGenerator }); if (current === session.current) update('password', data.password); }
    catch (error) { if (current === session.current) setError(errorMessage(error)); }
  };
  const save = async (event: React.FormEvent) => {
    event.preventDefault();
    if (!validateEntry(fields.path)) { setError('请输入有效名称，可用「文件夹/账号」分类；不能含隐藏路径或特殊分隔符。'); return; }
    if (!fields.password || /[\r\n]/.test(fields.password)) { setError('密码不能为空，也不能包含换行。'); return; }
    if (/[\r\n]/.test(fields.username + fields.url)) { setError('用户名和网站不能包含换行。'); return; }
    setSaving(true); setError('');
    try {
      await client.request({ action: 'save', vaultId: value?.vaultId ?? '', entry: value?.original?.entry.path ?? fields.path, destination: value?.original && value.original.entry.path !== fields.path ? fields.path : undefined,
        mode: value?.original ? 'edit' : 'create', revision: value?.original?.revision, contents: updateDocument(value?.original?.contents ?? '', fields) });
      await saved();
    } catch (error) { setError(errorMessage(error)); } finally { setSaving(false); }
  };
  return <Dialog.Root open={!!value} onOpenChange={open => { if (!open && !saving) close(); }}><Dialog.Content maxWidth="380px" className="editor-dialog" aria-describedby={undefined}><Dialog.Title>{value?.original ? '编辑账号' : '新建账号'}</Dialog.Title><form onSubmit={event => void save(event)}><Flex direction="column" gap="3">
    <label className="field-label">名称<TextField.Root autoFocus required value={fields.path} placeholder="github.com/个人账号" onChange={event => update('path', event.target.value)}/></label>
    <label className="field-label">用户名<TextField.Root value={fields.username} autoComplete="off" placeholder="用户名或邮箱" onChange={event => update('username', event.target.value)}/></label>
    <label className="field-label">密码<TextField.Root required type={reveal ? 'text' : 'password'} value={fields.password} autoComplete="new-password" onChange={event => update('password', event.target.value)}><TextField.Slot side="right"><IconButton type="button" aria-label="显示或隐藏密码" variant="ghost" size="1" onClick={() => setReveal(!reveal)}>{reveal ? <EyeOff size={15}/> : <Eye size={15}/>}</IconButton></TextField.Slot><TextField.Slot side="right"><IconButton type="button" aria-label="生成新密码" variant="ghost" size="1" onClick={() => void generate()}><Shuffle size={15}/></IconButton></TextField.Slot></TextField.Root></label>
    <label className="field-label">网站<TextField.Root value={fields.url} placeholder="https://example.com" onChange={event => update('url', event.target.value)}/></label>
    <label className="field-label">备注<TextArea value={fields.notes} placeholder="添加备注…" rows={2} onChange={event => update('notes', event.target.value)}/></label>
    {error && <Callout.Root color="red" size="1"><Callout.Text>{error}</Callout.Text></Callout.Root>}
    <Flex gap="3" justify="end" mt="2"><Button type="button" color="gray" variant="soft" disabled={saving} onClick={close}>取消</Button><Button type="submit" loading={saving}>保存</Button></Flex>
  </Flex></form></Dialog.Content></Dialog.Root>;
}
