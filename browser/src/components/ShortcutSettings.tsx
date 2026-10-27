import { useCopy, useLanguage } from './LanguageProvider';
import { useEffect, useRef, useState } from 'react';
import { Button, Callout, IconButton, Select, Text } from '@radix-ui/themes';
import { Plus, RotateCcw, Trash2 } from 'lucide-react';
import { browser } from 'wxt/browser';
import { assignedShortcuts, canonicalShortcut, defaultShortcuts, saveShortcut, shortcutDefinitions, type ShortcutAPI } from '../lib/shortcuts';
import { defaultPopupShortcuts, isMac, popupShortcutActions, readPopupShortcuts, writePopupShortcuts, type PopupShortcut, type PopupShortcutAction } from '../lib/popup-shortcuts';
import { defaultPageShortcuts, hasPageShortcutAccess, pageShortcutAccessKey, pageShortcutKey, pageShortcutOrigins, readPageShortcuts, writePageShortcuts, type PageShortcuts } from '../lib/page-shortcuts';
import { ShortcutRecorder } from './ShortcutRecorder';
import { SettingsGroup, SettingsRow } from './SettingsLayout';
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
  const [browserBindings, setBrowserBindings] = useState<Record<string, string>>({}), [access, setAccess] = useState(false);
  const [loaded, setLoaded] = useState(false), [readFailed, setReadFailed] = useState(false), [reading, setReading] = useState(true), [recorderVersion, setRecorderVersion] = useState(0);
  const [local, setLocal] = useState<PopupShortcut[]>(defaultPopupShortcuts);
  const [error, setError] = useState(''), [notice, setNotice] = useState(''), [saving, setSaving] = useState(false);
  useEffect(() => { setError(''); setNotice(''); }, [locale]);
  const sequence = useRef(0), mutating = useRef(false), refreshPending = useRef(false), active = useRef(false);
  const refresh = async () => {
    if (mutating.current) { refreshPending.current = true; return; }
    setReading(true);
    const current = ++sequence.current;
    const commandsPromise = demo ? Promise.resolve(defaultShortcuts(isMac(), firefox)) : api!.getAll().then(assignedShortcuts);
    const [commands, popup, page, permission] = await Promise.allSettled([
      commandsPromise, readPopupShortcuts(demo),
      editable ? commandsPromise : readPageShortcuts(demo),
      demo ? Promise.resolve(localStorage.getItem(pageShortcutAccessKey) === 'true') : editable ? Promise.resolve(true) : hasPageShortcutAccess(),
    ]);
    if (current !== sequence.current) return;
    if (commands.status === 'fulfilled') setBrowserBindings(commands.value);
    if (page.status === 'fulfilled') setBindings(page.value);
    if (popup.status === 'fulfilled') setLocal(popup.value);
    if (permission.status === 'fulfilled') setAccess(permission.value);
    const failed = [commands, popup, page, permission].some(value => value.status === 'rejected');
    setLoaded(!failed); setReadFailed(failed);
    setReading(false);
  };
  useEffect(() => {
    active.current = true;
    void refresh();
    const visible = () => { if (!document.hidden) void refresh(); };
    const changed = (changes: Record<string, unknown>, area: string) => { if (area === 'local' && (pageShortcutKey in changes || pageShortcutAccessKey in changes || 'popupShortcuts' in changes)) void refresh(); };
    const permissionsChanged = () => void refresh();
    window.addEventListener('focus', visible); document.addEventListener('visibilitychange', visible);
    if (!demo) { browser.storage.onChanged.addListener(changed); browser.permissions.onAdded.addListener(permissionsChanged); browser.permissions.onRemoved.addListener(permissionsChanged); }
    return () => { active.current = false; sequence.current++; window.removeEventListener('focus', visible); document.removeEventListener('visibilitychange', visible); if (!demo) { browser.storage.onChanged.removeListener(changed); browser.permissions.onAdded.removeListener(permissionsChanged); browser.permissions.onRemoved.removeListener(permissionsChanged); } };
  }, [demo, firefox]);
  const mutate = async (operation: () => Promise<void>) => {
    if (mutating.current) return;
    mutating.current = true; setSaving(true); setError(''); setNotice('');
    const current = ++sequence.current;
    try { await operation(); if (current === sequence.current) setNotice(copy.shortcuts.saved); }
    catch (error) { if (current === sequence.current) setError(error instanceof Error ? error.message : copy.shortcuts.saveFailed); }
    finally {
      mutating.current = false;
      if (current === sequence.current) setSaving(false);
      if (refreshPending.current) { refreshPending.current = false; if (active.current) void refresh(); }
    }
  };
  const saveLocal = (next: PopupShortcut[]) => void mutate(async () => {
    const keys = next.map(item => canonicalShortcut(item.shortcut)).filter(Boolean);
    if (new Set(keys).size !== keys.length || keys.some(key => [...Object.values(browserBindings), ...Object.values(bindings ?? {})].some(value => value && canonicalShortcut(value) === key))) throw new Error(copy.shortcuts.conflict);
    await writePopupShortcuts(next, demo); setLocal(next);
  });
  const invalid = () => setError(copy.shortcuts.invalid);
  const resetPopup = () => { setRecorderVersion(value => value + 1); saveLocal(defaultPopupShortcuts()); };
  const resetBrowser = () => {
    setRecorderVersion(value => value + 1);
    void mutate(async () => {
      const defaults = editable ? defaultShortcuts(isMac(), firefox) : defaultPageShortcuts(isMac());
      if (local.some(item => item.shortcut && Object.values(defaults).some(value => canonicalShortcut(value) === canonicalShortcut(item.shortcut)))) throw new Error(copy.shortcuts.popupConflict);
      if (!editable && Object.entries(defaults).some(([name, shortcut]) => Object.entries(browserBindings).some(([id, value]) => id !== name && value && canonicalShortcut(value) === canonicalShortcut(shortcut)))) throw new Error(copy.shortcuts.conflict);
      if (editable) {
        try { for (const { name } of commandDefinitions) await api!.reset!(name); }
        finally { const next = assignedShortcuts(await api!.getAll()); setBindings(next); setBrowserBindings(next); }
      } else { await writePageShortcuts(defaults, demo); setBindings(defaults); }
    });
  };
  const changePageShortcut = (name: string, shortcut: string) => void mutate(async () => {
    if (shortcut && [...local.map(item => item.shortcut), ...Object.entries(bindings ?? {}).filter(([id]) => id !== name).map(([,value]) => value), ...Object.entries(browserBindings).filter(([id]) => id !== name).map(([,value]) => value)].some(value => value && canonicalShortcut(value) === canonicalShortcut(shortcut))) throw new Error(copy.shortcuts.conflict);
    if (editable) { const next = await saveShortcut(api!, name, shortcut, copy); setBindings(next); setBrowserBindings(next); }
    else { const next = { ...bindings, [name]: shortcut } as PageShortcuts; await writePageShortcuts(next, demo); setBindings(next); }
  });
  const changeAccess = () => {
    if (mutating.current || !loaded) return;
    setError('');
    // Call request directly from this click, before awaiting storage or anything
    // else, to preserve the browser's required user gesture.
    const operation = demo ? Promise.resolve(true) : access ? browser.permissions.remove({ origins: pageShortcutOrigins }) : browser.permissions.request({ origins: pageShortcutOrigins });
    void mutate(async () => {
      let granted: boolean;
      try { granted = await operation; }
      catch { throw new Error(copy.shortcuts.permissionFailed); }
      if (!granted) throw new Error(copy.shortcuts.permissionDenied);
      try {
        const enabled = !access;
        if (demo) localStorage.setItem(pageShortcutAccessKey, String(enabled));
        setAccess(enabled);
      } catch { throw new Error(copy.shortcuts.permissionFailed); }
    });
  };
  return <div className="settings-shortcuts">
    <div className="shortcut-section">
      <SettingsGroup caption={editable ? copy.shortcuts.browserShortcuts : copy.shortcuts.pageShortcuts}>
        {commandDefinitions.map(({ name, label }) => <SettingsRow key={name} label={label} control={<ShortcutRecorder key={recorderVersion} command={editable} label={label} value={bindings?.[name] ?? ''} disabled={!loaded || saving} invalid={invalid} change={shortcut => changePageShortcut(name, shortcut)}/>}/>)}
        {!editable && <SettingsRow label={access ? copy.shortcuts.pageEnabled : copy.shortcuts.siteAccess} description={access ? undefined : copy.shortcuts.accessDescription} control={<Button size="1" variant="outline" color="gray" disabled={!loaded || saving} onClick={changeAccess}>{access ? copy.shortcuts.disablePageShortcuts : copy.shortcuts.enablePageShortcuts}</Button>}/>}
        <div className="settings-group-actions"><Button size="1" variant="ghost" color="gray" aria-label={editable ? copy.shortcuts.restoreBrowserDefaults : copy.shortcuts.restorePageDefaults} disabled={!loaded || saving} onClick={resetBrowser}><RotateCcw size={13}/>{copy.shortcuts.restoreDefaults}</Button></div>
      </SettingsGroup>
      <Text as="p" size="1" color="gray" className="settings-hint">{editable ? copy.shortcuts.browserEditable : copy.shortcuts.pageDescription}</Text>
    </div>
    <div className="shortcut-section">
      <SettingsGroup caption={copy.shortcuts.popupShortcuts}>
        {local.map((item, index) => {
          const label = popupActions.find(({ action }) => action === item.action)!.label;
          return <SettingsRow key={item.id} className="popup-shortcut-row" label={<Select.Root value={item.action} disabled={!loaded || saving} onValueChange={action => saveLocal(local.map(row => row.id === item.id ? { ...row, action: action as PopupShortcutAction } : row))}>
            <Select.Trigger aria-label={copy.shortcuts.action(index + 1)} variant="ghost"/><Select.Content>{popupActions.map(action => <Select.Item key={action.action} value={action.action}>{action.label}</Select.Item>)}</Select.Content>
          </Select.Root>} control={<div className="popup-shortcut-controls">
            <ShortcutRecorder key={recorderVersion} label={label} value={item.shortcut} disabled={!loaded || saving} invalid={invalid} change={shortcut => saveLocal(local.map(row => row.id === item.id ? { ...row, shortcut } : row))}/>
            <IconButton aria-label={copy.shortcuts.delete(label)} size="1" variant="ghost" color="gray" disabled={!loaded || saving} onClick={() => saveLocal(local.filter(row => row.id !== item.id))}><Trash2 size={14}/></IconButton>
          </div>}/>;
        })}
        <div className="settings-group-actions shortcut-bottom-actions">
          <Button size="1" variant="ghost" disabled={!loaded || saving} onClick={() => { const action = popupActions.find(action => !local.some(item => item.action === action.action))?.action ?? 'search'; saveLocal([...local, { id: crypto.randomUUID(), action, shortcut: '' }]); }}><Plus size={14}/>{copy.shortcuts.add}</Button>
          <Button size="1" variant="ghost" color="gray" aria-label={copy.shortcuts.restorePopupDefaults} disabled={!loaded || saving} onClick={resetPopup}><RotateCcw size={13}/>{copy.shortcuts.restoreDefaults}</Button>
        </div>
      </SettingsGroup>
      <Text as="p" size="1" color="gray" className="settings-hint">{copy.shortcuts.popupDescription}</Text>
    </div>
    <details className="settings-details shortcut-help"><summary>{copy.shortcuts.helpTitle}</summary><div className="shortcut-help-content">
      <Text as="p" size="1" color="gray">{copy.shortcuts.recordingHelp}</Text><Text as="p" size="1" color="gray">{copy.shortcuts.platformHelp}</Text><Text as="p" size="1" color="gray">{copy.shortcuts.contextHelp}</Text>
    </div></details>
    {(error || readFailed) && <Callout.Root size="1" color="amber" role="alert"><Callout.Text>{readFailed ? copy.shortcuts.loadingFailed : error}</Callout.Text>{readFailed && <Button size="1" variant="soft" color="gray" disabled={reading} aria-busy={reading || undefined} onClick={() => void refresh()}>{copy.shortcuts.retry}</Button>}</Callout.Root>}
    <Text as="p" size="1" color="gray" className="shortcut-status" role="status">{saving ? copy.shortcuts.saving : notice}</Text>
  </div>;
}
