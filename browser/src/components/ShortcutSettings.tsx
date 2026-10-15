import { useEffect, useRef, useState } from 'react';
import { Button, Callout, Heading, IconButton, Select, Text } from '@radix-ui/themes';
import { Command, ExternalLink, Plus, Trash2 } from 'lucide-react';
import { browser } from 'wxt/browser';
import { assignedShortcuts, canonicalShortcut, defaultShortcuts, saveShortcut, shortcutDefinitions, shortcutsPageURL, type ShortcutAPI } from '../lib/shortcuts';
import { defaultPopupShortcuts, isMac, popupShortcutActions, readPopupShortcuts, writePopupShortcuts, type PopupShortcut, type PopupShortcutAction } from '../lib/popup-shortcuts';
import { ShortcutKeys, ShortcutRecorder } from './ShortcutRecorder';
import './options-shortcuts.css';

export function ShortcutSettings({ firefox, demo = false }: { firefox: boolean; demo?: boolean }) {
  const api = browser?.commands as ShortcutAPI | undefined;
  const editable = !demo && firefox && !!api?.update && !!api?.reset;
  const [bindings, setBindings] = useState<Record<string, string>>();
  const [local, setLocal] = useState<PopupShortcut[]>(defaultPopupShortcuts);
  const [error, setError] = useState(''), [notice, setNotice] = useState(''), [saving, setSaving] = useState(false);
  const sequence = useRef(0), mutating = useRef(false);
  const refresh = async () => {
    if (mutating.current) return;
    const current = ++sequence.current;
    const [commands, popup] = await Promise.allSettled([demo ? Promise.resolve(defaultShortcuts(isMac())) : api!.getAll().then(assignedShortcuts), readPopupShortcuts(demo)]);
    if (current !== sequence.current) return;
    if (commands.status === 'fulfilled') setBindings(commands.value);
    if (popup.status === 'fulfilled') setLocal(popup.value);
    setError(commands.status === 'rejected' || popup.status === 'rejected' ? '部分快捷键读取失败，请重新打开设置页。' : '');
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
    try { await operation(); if (current === sequence.current) setNotice('快捷键已保存'); }
    catch (error) { if (current === sequence.current) setError(error instanceof Error ? error.message : '快捷键未保存，请重试。'); }
    finally { mutating.current = false; if (current === sequence.current) setSaving(false); }
  };
  const openSettings = async () => {
    setError('');
    try { if (firefox) await api!.openShortcutSettings!(); else await browser.tabs.create({ url: shortcutsPageURL(navigator.userAgent) }); }
    catch { setError('无法打开浏览器快捷键设置，请从扩展管理页面打开快捷键设置。'); }
  };
  const saveLocal = (next: PopupShortcut[]) => void mutate(async () => {
    const keys = next.map(item => canonicalShortcut(item.shortcut)).filter(Boolean);
    if (new Set(keys).size !== keys.length || keys.some(key => Object.values(bindings ?? {}).some(value => value && canonicalShortcut(value) === key))) throw new Error('此快捷键已用于另一项操作，请选择其他组合。');
    await writePopupShortcuts(next, demo); setLocal(next);
  });
  const invalid = () => setError('请使用 Ctrl、Command 或 Alt 与字母、数字等按键的组合。');
  return <>
    <div className="options-section-heading"><Command size={18}/><Heading as="h2" size="3">快捷键</Heading></div>
    <Text as="div" size="1" color="gray" className="shortcut-group-title">浏览器快捷键</Text>
    <dl className="shortcut-list shortcut-settings-list">{shortcutDefinitions.map(({ name, label }) => <div key={name}><dt>{label}</dt><dd>
      {editable ? <ShortcutRecorder command label={label} value={bindings?.[name] ?? ''} disabled={!bindings || saving} invalid={invalid} change={shortcut => void mutate(async () => {
        if (shortcut && local.some(item => canonicalShortcut(item.shortcut) === canonicalShortcut(shortcut))) throw new Error('此快捷键已用于弹窗中的另一项操作。');
        setBindings(await saveShortcut(api!, name, shortcut));
      })}/> : <button type="button" className="shortcut-recorder shortcut-system-button" aria-label={`修改${label}快捷键`} disabled={demo || !bindings} onClick={() => void openSettings()}>{bindings?.[name] ? <ShortcutKeys value={bindings[name]!}/> : <span className="shortcut-placeholder">{bindings ? '未设置' : '正在读取…'}</span>}<ExternalLink size={12}/></button>}
    </dd></div>)}</dl>
    <div className="shortcut-group-heading"><Text size="1" color="gray">弹窗快捷键</Text><Text size="1" color="gray">弹窗打开时可用</Text></div>
    <div className="popup-shortcut-list">{local.map((item, index) => {
      const label = popupShortcutActions.find(({ action }) => action === item.action)!.label;
      return <div className="popup-shortcut-row" key={item.id}><Select.Root value={item.action} disabled={saving} onValueChange={action => saveLocal(local.map(row => row.id === item.id ? { ...row, action: action as PopupShortcutAction } : row))}><Select.Trigger aria-label={`快捷键 ${index + 1} 的操作`} variant="ghost"/><Select.Content>{popupShortcutActions.map(action => <Select.Item key={action.action} value={action.action}>{action.label}</Select.Item>)}</Select.Content></Select.Root><div className="popup-shortcut-controls"><ShortcutRecorder label={label} value={item.shortcut} disabled={saving} invalid={invalid} change={shortcut => saveLocal(local.map(row => row.id === item.id ? { ...row, shortcut } : row))}/><IconButton aria-label={`删除${label}快捷键`} size="1" variant="ghost" color="gray" disabled={saving} onClick={() => saveLocal(local.filter(row => row.id !== item.id))}><Trash2 size={14}/></IconButton></div></div>;
    })}</div>
    <div className="shortcut-bottom-actions"><Button size="1" variant="ghost" disabled={saving} onClick={() => { const action = popupShortcutActions.find(action => !local.some(item => item.action === action.action))?.action ?? 'search'; saveLocal([...local, { id: crypto.randomUUID(), action, shortcut: '' }]); }}><Plus size={14}/>添加快捷键</Button><Button size="1" variant="ghost" color="gray" disabled={saving} onClick={() => saveLocal(defaultPopupShortcuts())}>恢复弹窗默认</Button></div>
    <Text as="p" size="1" color="gray" className="shortcut-recording-help">点击键帽录制，按 Esc 取消，Delete 或 Backspace 清除。</Text>
    {error && <Callout.Root mt="3" size="1" color="amber" role="alert"><Callout.Text>{error}</Callout.Text></Callout.Root>}
    {notice && <Text as="p" size="1" color="gray" className="shortcut-recording-help" role="status">{notice}</Text>}
  </>;
}
