import { useCopy, useLanguage } from './LanguageProvider';
import { useEffect, useRef, useState } from 'react';
import { AlertDialog, Button, Callout, Flex, IconButton, Text, TextField } from '@radix-ui/themes';
import { Eye, EyeOff, Plus, RefreshCw, Trash2 } from 'lucide-react';
import { additionalFields, isPrimaryFieldKey, parseDocument, updateDocument, validateEntry } from '../lib/model';
import { errorMessage } from '../lib/errors';
import type { Client } from '../lib/types';
import type { EditorValue } from './App';
import { PageHeader } from './Primitives';
import { Generator } from './Generator';

export function EditorForm({ client, value, defaultURL, close, saved }: { client: Client; value: EditorValue; defaultURL: string; close(): void; saved(): Promise<void> }) {
  const copy = useCopy();
  const { locale } = useLanguage();
  const [fields, setFields] = useState({ path: '', username: '', password: '', url: '', notes: '' });
  const [customFields, setCustomFields] = useState<Array<{ id: number; sourceIndex: number; key: string; value: string }>>([]);
  const [generatorOpen, setGeneratorOpen] = useState(false);
  const [error, setError] = useState(''), [saving, setSaving] = useState(false), [reveal, setReveal] = useState(false);
  useEffect(() => { setError(''); }, [locale]);
  const session = useRef(0), savingRequest = useRef(false), defaultWebsite = useRef(defaultURL), nextField = useRef(0);
  const [leaveOpen, setLeaveOpen] = useState(false);
  const editorForm = useRef<HTMLFormElement>(null);
  const initialForm = useRef(''), lastField = useRef<HTMLInputElement | null>(null), discarded = useRef(false);
  const returnFocus = useRef<{ field: HTMLInputElement | null; start: number | null; end: number | null; direction: 'forward' | 'backward' | 'none' | null }>({ field: null, start: null, end: null, direction: null });
  const fingerprint = (nextFields: typeof fields, extraFields: typeof customFields) => JSON.stringify({ fields: nextFields, custom: extraFields.filter(field => field.key || field.value).map(({ key, value }) => ({ key, value })) });
  const requestLeave = () => {
    if (savingRequest.current) return;
    if (fingerprint(fields, customFields) === initialForm.current) { close(); return; }
    const field = lastField.current;
    returnFocus.current = { field, start: field?.selectionStart ?? null, end: field?.selectionEnd ?? null, direction: field?.selectionDirection ?? null };
    discarded.current = false; setLeaveOpen(true);
  };
  const passwordInput = useRef<HTMLInputElement>(null), wasGenerating = useRef(false);
  defaultWebsite.current = defaultURL;
  useEffect(() => {
    session.current++;
    const doc = value.original ? parseDocument(value.original.contents) : undefined;
    const nextFields = { path: value.original?.entry.path ?? '', username: doc?.username ?? '', password: doc?.password ?? value.password ?? '', url: doc?.urls[0] ?? defaultWebsite.current, notes: doc?.notes ?? '' };
    const nextCustom = doc ? additionalFields(doc).map((field, sourceIndex) => ({ ...field, sourceIndex, id: nextField.current++ })) : [];
    initialForm.current = fingerprint(nextFields, nextCustom);
    setFields(nextFields); setCustomFields(nextCustom); setLeaveOpen(false);
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
    if (!validateEntry(fields.path)) { setError(copy.editor.invalidName); return; }
    if (!fields.password || /[\r\n]/.test(fields.password)) { setError(copy.editor.invalidPassword); return; }
    if (/[\r\n]/.test(fields.username + fields.url)) { setError(copy.editor.invalidPrimaryFields); return; }
    const extra = customFields.filter(field => field.key || field.value).map(({ key, value, sourceIndex }) => ({ key, value, sourceIndex }));
    if (extra.some(field => ['note', 'notes'].includes(field.key.toLowerCase()))) { setError(copy.editor.reservedNotes); return; }
    const originalFields = value.original ? additionalFields(parseDocument(value.original.contents)) : [];
    const reserved = extra.find(field => isPrimaryFieldKey(field.key) && originalFields[field.sourceIndex]?.key !== field.key);
    if (reserved) { setError(copy.editor.reservedField(reserved.key)); return; }
    if (extra.some(field => !/^[\p{L}\p{Nd}_-]+$/u.test(field.key) || /[\r\n]/.test(field.value))) { setError(copy.editor.invalidCustomField); return; }
    const current = session.current;
    savingRequest.current = true;
    setSaving(true); setError('');
    try {
      await client.request({ action: 'save', vaultId: value.vaultId, entry: value.original?.entry.path ?? fields.path, destination: value.original && value.original.entry.path !== fields.path ? fields.path : undefined,
        mode: value.original ? 'edit' : 'create', revision: value.original?.revision, contents: updateDocument(value.original?.contents ?? '', { ...fields, customFields: extra }) });
      if (current === session.current) await saved();
    } catch (error) { if (current === session.current) setError(errorMessage(error, copy)); }
    finally { savingRequest.current = false; if (current === session.current) setSaving(false); }
  };
  return <><form ref={editorForm} className="popup-screen editor-screen" inert={generatorOpen} aria-hidden={generatorOpen} onSubmit={event => void save(event)} onFocusCapture={event => { if (event.target instanceof HTMLInputElement) lastField.current = event.target; }}>
    <PageHeader title={value.original ? copy.accounts.edit : copy.accounts.create} backLabel={value.original ? copy.editor.backToDetails : copy.common.backToVault} back={requestLeave} disabled={saving}/>
    <main className="screen-content editor-content">
      <fieldset className="editor-fields" disabled={saving}>
        <label className="field-label">{copy.editor.name}<TextField.Root autoFocus required value={fields.path} placeholder={copy.editor.pathPlaceholder} onChange={event => update('path', event.target.value)}/></label>
        <label className="field-label">{copy.common.username}<TextField.Root value={fields.username} autoComplete="off" placeholder={copy.editor.usernamePlaceholder} onChange={event => update('username', event.target.value)}/></label>
        <div className="field-label"><label htmlFor="entry-password">{copy.common.password}</label><TextField.Root ref={passwordInput} id="entry-password" required type={reveal ? 'text' : 'password'} value={fields.password} autoComplete="new-password" onChange={event => update('password', event.target.value)}><TextField.Slot side="right"><IconButton type="button" aria-label={reveal ? copy.common.hidePassword : copy.common.showPassword} variant="ghost" size="1" onClick={() => setReveal(!reveal)}>{reveal ? <EyeOff size={15}/> : <Eye size={15}/>}</IconButton></TextField.Slot></TextField.Root><Button className="editor-generate-button" type="button" variant="ghost" size="1" onClick={() => setGeneratorOpen(true)}><RefreshCw size={13}/>{copy.generator.regeneratePassword}</Button></div>
        <label className="field-label">{copy.common.website}<TextField.Root value={fields.url} autoComplete="url" placeholder={copy.editor.websitePlaceholder} onChange={event => update('url', event.target.value)}/></label>
        <div className="custom-fields"><div className="custom-fields-heading"><Text size="2" weight="medium">{copy.editor.customFields}</Text><Button type="button" variant="ghost" size="1" onClick={() => setCustomFields(previous => [...previous, { id: nextField.current++, sourceIndex: -1, key: '', value: '' }])}><Plus size={13}/>{copy.editor.addField}</Button></div>{customFields.map((field, index) => <div className="custom-field-row" key={field.id}><TextField.Root aria-label={copy.editor.fieldName(index + 1)} value={field.key} placeholder={copy.editor.fieldKeyPlaceholder} onChange={event => setCustomFields(previous => previous.map(item => item.id === field.id ? { ...item, key: event.target.value } : item))}/><TextField.Root aria-label={copy.editor.fieldValue(index + 1)} value={field.value} placeholder={copy.editor.fieldValuePlaceholder} onChange={event => setCustomFields(previous => previous.map(item => item.id === field.id ? { ...item, value: event.target.value } : item))}/><IconButton type="button" aria-label={copy.editor.deleteField(index + 1)} variant="ghost" color="gray" onClick={() => setCustomFields(previous => previous.filter(item => item.id !== field.id))}><Trash2 size={15}/></IconButton></div>)}</div>
      </fieldset>
    </main>
    {error && <Callout.Root color="red" size="1" role="alert" className="screen-error"><Callout.Text>{error}</Callout.Text></Callout.Root>}
    <footer className="screen-footer"><Button type="button" color="gray" variant="soft" disabled={saving} onClick={requestLeave}>{copy.common.cancel}</Button><Button type="submit" className="grow" loading={saving}>{copy.common.save}</Button></footer>
  </form><Generator client={client} open={generatorOpen} close={() => setGeneratorOpen(false)} usePassword={password => { update('password', password); setGeneratorOpen(false); }} canSave useLabel={copy.generator.usePassword} backLabel={copy.editor.backToEditor}/>
    <AlertDialog.Root open={leaveOpen} onOpenChange={setLeaveOpen}>
      <AlertDialog.Content className="leave-dialog" maxWidth="340px" onCloseAutoFocus={event => {
        if (discarded.current) return;
        event.preventDefault();
        requestAnimationFrame(() => {
          const { field, start, end, direction } = returnFocus.current;
          if (!field?.isConnected) { editorForm.current?.querySelector<HTMLInputElement>('input')?.focus({ preventScroll: true }); return; }
          field.focus({ preventScroll: true });
          if (start !== null && end !== null) field.setSelectionRange(start, end, direction ?? undefined);
        });
      }}>
        <AlertDialog.Title>{copy.editor.unsavedTitle}</AlertDialog.Title>
        <AlertDialog.Description size="2">{copy.editor.unsavedDescription}</AlertDialog.Description>
        <Flex justify="end" gap="2" mt="5" wrap="wrap">
          <AlertDialog.Action><Button type="button" variant="soft" color="gray" onClick={() => { discarded.current = true; close(); }}>{copy.editor.discardChanges}</Button></AlertDialog.Action>
          <AlertDialog.Cancel><Button type="button">{copy.editor.continueEditing}</Button></AlertDialog.Cancel>
        </Flex>
      </AlertDialog.Content>
    </AlertDialog.Root></>;
}
