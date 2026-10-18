import { useCopy, useLanguage } from './LanguageProvider';
import { useEffect, useRef, useState } from 'react';
import { Button, Callout, Heading, IconButton, Select, Text } from '@radix-ui/themes';
import { Command, ExternalLink, Plus, Trash2 } from 'lucide-react';
import { browser } from 'wxt/browser';
import { assignedShortcuts, canonicalShortcut, defaultShortcuts, saveShortcut, shortcutDefinitions, shortcutsPageURL, type ShortcutAPI } from '../lib/shortcuts';
import { defaultPopupShortcuts, isMac, popupShortcutActions, readPopupShortcuts, writePopupShortcuts, type PopupShortcut, type PopupShortcutAction } from '../lib/popup-shortcuts';
import { ShortcutKeys, ShortcutRecorder } from './ShortcutRecorder';
import './options-shortcuts.css';

export function ShortcutSettings({ firefox, demo = false }: { firefox: boolean; demo?: boolean }) {
  const copy = useCopy();
  const { locale } = useLanguage();
  const commandDefinitions = shortcutDefinitions.map(item => ({ ...item, label: copy.shortcuts[item.labelKey] }));
  const popupActions = popupShortcutActions.map(item => ({ ...item, label: ({
    search: copy.accounts.search, new: copy.accounts.create, refresh: copy.vault.refreshVault, lock: copy.vault.lock,
    settings: copy.shortcuts.openSettings, copy: copy.shortcuts.copyCurrentPassword, fill: copy.shortcuts.fillCurrent, edit: copy.shortcuts.editCurrent,
  })[item.action] }));
  const api = browser?.commands as ShortcutAPI | undefined;
  const editable = !demo && firefox && !!api?.update && !!api?.reset;
  const [bindings, setBindings] = useState<Record<string, string>>();
  const [local, setLocal] = useState<PopupShortcut[]>(defaultPopupShortcuts);
  const [error, setError] = useState(''), [notice, setNotice] = useState(''), [saving, setSaving] = useState(false);
  useEffect(() => { setError(''); setNotice(''); }, [locale]);
  const sequence = useRef(0), mutating = useRef(false);
  const refresh = async () => {
    if (mutating.current) return;
    const current = ++sequence.current;
    const [commands, popup] = await Promise.allSettled([demo ? Promise.resolve(defaultShortcuts(isMac())) : api!.getAll().then(assignedShortcuts), readPopupShortcuts(demo)]);
    if (current !== sequence.current) return;
    if (commands.status === 'fulfilled') setBindings(commands.value);
    if (popup.status === 'fulfilled') setLocal(popup.value);
    setError(commands.status === 'rejected' || popup.status === 'rejected' ? copy.shortcuts.readFailed : '');
  };
  useEffect(() => {
    void refresh();
    const visible = () => { if (!document.hidden) void refresh(); };
    window.addEventListener('focus', visible); document.addEventListener('visibilitychange', visible);
    return () => { sequence.current++; window.removeEventListener('focus', visible); document.removeEventListener('visibilitychange', visible); };
  }, [demo, firefox]);
  const mutate = async (operation: () => Promise<void>) => {
    if (mutating.current) return;
    mutating.current = true; setSaving(true); setError(''); setNotice('');
    const current = ++sequence.current;
    try { await operation(); if (current === sequence.current) setNotice(copy.shortcuts.saved); }
    catch (error) { if (current === sequence.current) setError(error instanceof Error ? error.message : copy.shortcuts.saveFailed); }
    finally { mutating.current = false; if (current === sequence.current) setSaving(false); }
  };
  const openSettings = async () => {
    setError('');
    try { if (firefox) await api!.openShortcutSettings!(); else await browser.tabs.create({ url: shortcutsPageURL(navigator.userAgent) }); }
    catch { setError(copy.shortcuts.openSettingsFailed); }
  };
  const saveLocal = (next: PopupShortcut[]) => void mutate(async () => {
    const keys = next.map(item => canonicalShortcut(item.shortcut)).filter(Boolean);
    if (new Set(keys).size !== keys.length || keys.some(key => Object.values(bindings ?? {}).some(value => value && canonicalShortcut(value) === key))) throw new Error(copy.shortcuts.conflict);
    await writePopupShortcuts(next, demo); setLocal(next);
  });
  const invalid = () => setError(copy.shortcuts.invalid);
  return <>
    <div className="options-section-heading"><Command size={18}/><Heading as="h2" size="3">{copy.shortcuts.title}</Heading></div>
    <Text as="div" size="1" color="gray" className="shortcut-group-title">{copy.shortcuts.browserShortcuts}</Text>
    <dl className="shortcut-list shortcut-settings-list">{commandDefinitions.map(({ name, label }) => <div key={name}><dt>{label}</dt><dd>
      {editable ? <ShortcutRecorder command label={label} value={bindings?.[name] ?? ''} disabled={!bindings || saving} invalid={invalid} change={shortcut => void mutate(async () => {
        if (shortcut && local.some(item => canonicalShortcut(item.shortcut) === canonicalShortcut(shortcut))) throw new Error(copy.shortcuts.popupConflict);
        setBindings(await saveShortcut(api!, name, shortcut, copy));
      })}/> : <button type="button" className="shortcut-recorder shortcut-system-button" aria-label={copy.shortcuts.modify(label)} disabled={demo || !bindings} onClick={() => void openSettings()}>{bindings?.[name] ? <ShortcutKeys value={bindings[name]!}/> : <span className="shortcut-placeholder">{bindings ? copy.common.notSet : copy.shortcuts.reading}</span>}<ExternalLink size={12}/></button>}
    </dd></div>)}</dl>
    <div className="shortcut-group-heading"><Text size="1" color="gray">{copy.shortcuts.popupShortcuts}</Text><Text size="1" color="gray">{copy.shortcuts.popupAvailability}</Text></div>
    <div className="popup-shortcut-list">{local.map((item, index) => {
      const label = popupActions.find(({ action }) => action === item.action)!.label;
      return <div className="popup-shortcut-row" key={item.id}><Select.Root value={item.action} disabled={saving} onValueChange={action => saveLocal(local.map(row => row.id === item.id ? { ...row, action: action as PopupShortcutAction } : row))}><Select.Trigger aria-label={copy.shortcuts.action(index + 1)} variant="ghost"/><Select.Content>{popupActions.map(action => <Select.Item key={action.action} value={action.action}>{action.label}</Select.Item>)}</Select.Content></Select.Root><div className="popup-shortcut-controls"><ShortcutRecorder label={label} value={item.shortcut} disabled={saving} invalid={invalid} change={shortcut => saveLocal(local.map(row => row.id === item.id ? { ...row, shortcut } : row))}/><IconButton aria-label={copy.shortcuts.delete(label)} size="1" variant="ghost" color="gray" disabled={saving} onClick={() => saveLocal(local.filter(row => row.id !== item.id))}><Trash2 size={14}/></IconButton></div></div>;
    })}</div>
    <div className="shortcut-bottom-actions"><Button size="1" variant="ghost" disabled={saving} onClick={() => { const action = popupActions.find(action => !local.some(item => item.action === action.action))?.action ?? 'search'; saveLocal([...local, { id: crypto.randomUUID(), action, shortcut: '' }]); }}><Plus size={14}/>{copy.shortcuts.add}</Button><Button size="1" variant="ghost" color="gray" disabled={saving} onClick={() => saveLocal(defaultPopupShortcuts())}>{copy.shortcuts.restorePopupDefaults}</Button></div>
    <Text as="p" size="1" color="gray" className="shortcut-recording-help">{copy.shortcuts.recordingHelp}</Text>
    {error && <Callout.Root mt="3" size="1" color="amber" role="alert"><Callout.Text>{error}</Callout.Text></Callout.Root>}
    {notice && <Text as="p" size="1" color="gray" className="shortcut-recording-help" role="status">{notice}</Text>}
  </>;
}
