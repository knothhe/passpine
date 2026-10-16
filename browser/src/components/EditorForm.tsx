import { useEffect, useRef, useState } from 'react';
import { Button, Callout, IconButton, Text, TextField } from '@radix-ui/themes';
import { Eye, EyeOff, Plus, RefreshCw, Trash2 } from 'lucide-react';
import { additionalFields, isPrimaryFieldKey, parseDocument, updateDocument, validateEntry } from '../lib/model';
import { errorMessage } from '../lib/errors';
import type { Client } from '../lib/types';
import type { EditorValue } from './App';
import { PageHeader } from './Primitives';
import { Generator } from './Generator';

export function EditorForm({ client, value, defaultURL, close, saved }: { client: Client; value: EditorValue; defaultURL: string; close(): void; saved(): Promise<void> }) {
  const [fields, setFields] = useState({ path: '', username: '', password: '', url: '', notes: '' });
  const [customFields, setCustomFields] = useState<Array<{ id: number; sourceIndex: number; key: string; value: string }>>([]);
  const [generatorOpen, setGeneratorOpen] = useState(false);
  const [error, setError] = useState(''), [saving, setSaving] = useState(false), [reveal, setReveal] = useState(false);
  const session = useRef(0), savingRequest = useRef(false), defaultWebsite = useRef(defaultURL), nextField = useRef(0);
  const passwordInput = useRef<HTMLInputElement>(null), wasGenerating = useRef(false);
  defaultWebsite.current = defaultURL;
  useEffect(() => {
    session.current++;
    const doc = value.original ? parseDocument(value.original.contents) : undefined;
    setFields({ path: value.original?.entry.path ?? '', username: doc?.username ?? '', password: doc?.password ?? value.password ?? '', url: doc?.urls[0] ?? defaultWebsite.current, notes: doc?.notes ?? '' });
    setCustomFields(doc ? additionalFields(doc).map((field, sourceIndex) => ({ ...field, sourceIndex, id: nextField.current++ })) : []);
    setError(''); setReveal(false); setGeneratorOpen(false);
    return () => { session.current++; };
  }, [value]);
  useEffect(() => {
    if (!generatorOpen && wasGenerating.current) passwordInput.current?.focus({ preventScroll: true });
    wasGenerating.current = generatorOpen;
  }, [generatorOpen]);
  const update = (key: keyof typeof fields, next: string) => setFields(previous => ({ ...previous, [key]: next }));
  const save = async (event: React.FormEvent) => {
    event.preventDefault();
    if (savingRequest.current) return;
    if (!validateEntry(fields.path)) { setError('请输入有效名称，可用「文件夹/账号」分类；不能含隐藏路径或特殊分隔符。'); return; }
    if (!fields.password || /[\r\n]/.test(fields.password)) { setError('密码不能为空，也不能包含换行。'); return; }
    if (/[\r\n]/.test(fields.username + fields.url)) { setError('用户名和网站不能包含换行。'); return; }
    const extra = customFields.filter(field => field.key || field.value).map(({ key, value, sourceIndex }) => ({ key, value, sourceIndex }));
    if (extra.some(field => ['note', 'notes'].includes(field.key.toLowerCase()))) { setError('note 和 notes 是备注保留字段名，请使用其他自定义字段名称。'); return; }
    const originalFields = value.original ? additionalFields(parseDocument(value.original.contents)) : [];
    const reserved = extra.find(field => isPrimaryFieldKey(field.key) && originalFields[field.sourceIndex]?.key !== field.key);
    if (reserved) { setError(`${reserved.key} 是用户名或网站保留字段名，请使用上方对应输入框。`); return; }
    if (extra.some(field => !/^[\p{L}\p{Nd}_-]+$/u.test(field.key) || /[\r\n]/.test(field.value))) { setError('自定义字段名称只能包含文字、十进制数字、下划线或连字符，值不能包含换行。'); return; }
    const current = session.current;
    savingRequest.current = true;
    setSaving(true); setError('');
    try {
      await client.request({ action: 'save', vaultId: value.vaultId, entry: value.original?.entry.path ?? fields.path, destination: value.original && value.original.entry.path !== fields.path ? fields.path : undefined,
        mode: value.original ? 'edit' : 'create', revision: value.original?.revision, contents: updateDocument(value.original?.contents ?? '', { ...fields, customFields: extra }) });
      if (current === session.current) await saved();
    } catch (error) { if (current === session.current) setError(errorMessage(error)); }
    finally { savingRequest.current = false; if (current === session.current) setSaving(false); }
  };
  return <><form className="popup-screen editor-screen" inert={generatorOpen} aria-hidden={generatorOpen} onSubmit={event => void save(event)}>
    <PageHeader title={value.original ? '编辑账号' : '新建账号'} backLabel={value.original ? '返回账号详情' : '返回密码库'} back={close} disabled={saving}/>
    <main className="screen-content editor-content">
      <fieldset className="editor-fields" disabled={saving}>
        <label className="field-label">名称<TextField.Root autoFocus required value={fields.path} placeholder="path/to/entry（支持多级路径）" onChange={event => update('path', event.target.value)}/></label>
        <label className="field-label">用户名<TextField.Root value={fields.username} autoComplete="off" placeholder="用户名或邮箱" onChange={event => update('username', event.target.value)}/></label>
        <div className="field-label"><label htmlFor="entry-password">密码</label><TextField.Root ref={passwordInput} id="entry-password" required type={reveal ? 'text' : 'password'} value={fields.password} autoComplete="new-password" onChange={event => update('password', event.target.value)}><TextField.Slot side="right"><IconButton type="button" aria-label={reveal ? '隐藏密码' : '显示密码'} variant="ghost" size="1" onClick={() => setReveal(!reveal)}>{reveal ? <EyeOff size={15}/> : <Eye size={15}/>}</IconButton></TextField.Slot></TextField.Root><Button className="editor-generate-button" type="button" variant="ghost" size="1" onClick={() => setGeneratorOpen(true)}><RefreshCw size={13}/>重新生成密码</Button></div>
        <label className="field-label">网站<TextField.Root value={fields.url} autoComplete="url" placeholder="https://example.com" onChange={event => update('url', event.target.value)}/></label>
        <div className="custom-fields"><div className="custom-fields-heading"><Text size="2" weight="medium">自定义字段</Text><Button type="button" variant="ghost" size="1" onClick={() => setCustomFields(previous => [...previous, { id: nextField.current++, sourceIndex: -1, key: '', value: '' }])}><Plus size={13}/>添加字段</Button></div>{customFields.map((field, index) => <div className="custom-field-row" key={field.id}><TextField.Root aria-label={`字段 ${index + 1} 名称`} value={field.key} placeholder="key" onChange={event => setCustomFields(previous => previous.map(item => item.id === field.id ? { ...item, key: event.target.value } : item))}/><TextField.Root aria-label={`字段 ${index + 1} 值`} value={field.value} placeholder="value" onChange={event => setCustomFields(previous => previous.map(item => item.id === field.id ? { ...item, value: event.target.value } : item))}/><IconButton type="button" aria-label={`删除字段 ${index + 1}`} variant="ghost" color="gray" onClick={() => setCustomFields(previous => previous.filter(item => item.id !== field.id))}><Trash2 size={15}/></IconButton></div>)}</div>
      </fieldset>
    </main>
    {error && <Callout.Root color="red" size="1" role="alert" className="screen-error"><Callout.Text>{error}</Callout.Text></Callout.Root>}
    <footer className="screen-footer"><Button type="button" color="gray" variant="soft" disabled={saving} onClick={close}>取消</Button><Button type="submit" className="grow" loading={saving}>保存</Button></footer>
  </form><Generator client={client} open={generatorOpen} close={() => setGeneratorOpen(false)} usePassword={password => { update('password', password); setGeneratorOpen(false); }} canSave useLabel="使用此密码" backLabel="返回编辑账号"/></>;
}
