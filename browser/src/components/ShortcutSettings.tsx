import { useEffect, useRef, useState } from 'react';
import { Button, Callout, Heading, Text, TextField } from '@radix-ui/themes';
import { Command, ExternalLink, Pencil } from 'lucide-react';
import { browser } from 'wxt/browser';
import { assignedShortcuts, captureShortcut, defaultShortcuts, formatShortcut, saveShortcut, shortcutDefinitions, shortcutsPageURL, type ShortcutAPI } from '../lib/shortcuts';
import './options-shortcuts.css';

export function ShortcutSettings({ firefox, demo = false }: { firefox: boolean; demo?: boolean }) {
  const mac = /Mac/.test(navigator.userAgent);
  const api = browser?.commands as ShortcutAPI | undefined;
  const editable = firefox && !!api?.update && !!api?.reset;
  const [bindings, setBindings] = useState<Record<string, string>>();
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');
  const [editing, setEditing] = useState<string>();
  const [draft, setDraft] = useState('');
  const [saving, setSaving] = useState(false);
  const sequence = useRef(0);
  const mutating = useRef(false);
  const refresh = async () => {
    if (mutating.current) return;
    const current = ++sequence.current;
    try {
      const values = demo ? defaultShortcuts(mac) : assignedShortcuts(await api!.getAll());
      if (current === sequence.current) { setBindings(values); setError(''); }
    } catch {
      if (current === sequence.current) setError('无法读取当前快捷键，请重新打开设置页。');
    }
  };
  useEffect(() => {
    void refresh();
    const visible = () => { if (!document.hidden) void refresh(); };
    window.addEventListener('focus', visible);
    document.addEventListener('visibilitychange', visible);
    return () => { sequence.current++; window.removeEventListener('focus', visible); document.removeEventListener('visibilitychange', visible); };
  }, [demo, firefox]);

  const openSettings = async () => {
    setError('');
    try {
      if (firefox) await api!.openShortcutSettings!();
      else await browser.tabs.create({ url: shortcutsPageURL(navigator.userAgent) });
    } catch { setError('无法打开浏览器快捷键设置，请从扩展管理页面打开快捷键设置。'); }
  };
  const save = async (name: string, reset = false) => {
    if (!api || mutating.current) return;
    mutating.current = true;
    setSaving(true); setError(''); setNotice('');
    const current = ++sequence.current;
    try {
      const values = reset ? (await api.reset!(name), assignedShortcuts(await api.getAll())) : await saveShortcut(api, name, draft);
      if (current !== sequence.current) return;
      setBindings(values); setEditing(undefined); setNotice(reset ? '已恢复默认快捷键。' : '快捷键已保存。');
    } catch (error) { if (current === sequence.current) setError(error instanceof Error ? error.message : '快捷键未保存，请重试。'); }
    finally { mutating.current = false; if (current === sequence.current) setSaving(false); }
  };

  return <>
    <div className="options-section-heading"><Command size={18}/><Heading as="h2" size="3">快捷键</Heading></div>
    <dl className="shortcut-list shortcut-settings-list">{shortcutDefinitions.map(({ name, label }) => <div key={name}>
      <dt>{label}</dt><dd>
        {editing === name ? <div className="shortcut-editor">
          <TextField.Root autoFocus readOnly value={draft ? formatShortcut(draft) : ''} placeholder="按下快捷键组合" aria-label={`${label}快捷键`} aria-describedby="shortcut-recording-help" disabled={saving} onKeyDown={event => {
            if (event.key === 'Tab') return;
            event.preventDefault(); event.stopPropagation();
            const result = captureShortcut(event.nativeEvent, mac);
            if (result.type === 'cancel') { setEditing(undefined); setError(''); }
            else if (result.type === 'shortcut') { setDraft(result.shortcut); setError(''); }
            else if (result.type === 'invalid') setError('请按 Ctrl、Command 或 Alt 与字母、数字等按键的组合，最多使用两个修饰键。');
          }}/>
          <div className="shortcut-editor-actions"><Button size="1" disabled={saving} onClick={() => void save(name)}>保存</Button><Button size="1" variant="soft" color="gray" disabled={saving} onClick={() => { setEditing(undefined); setError(''); }}>取消</Button><Button size="1" variant="ghost" color="gray" disabled={saving} onClick={() => setDraft('')}>清除</Button></div>
        </div> : <div className="shortcut-binding"><kbd>{bindings ? bindings[name] ? formatShortcut(bindings[name]!) : '未设置' : error ? '读取失败' : '正在读取…'}</kbd>{editable && <Button size="1" variant="ghost" color="gray" disabled={!bindings || saving} aria-label={`修改${label}快捷键`} onClick={() => { setEditing(name); setDraft(bindings?.[name] ?? ''); setError(''); setNotice(''); }}><Pencil size={12}/>修改</Button>}</div>}
        {editable && <Button className="shortcut-reset" size="1" variant="ghost" color="gray" disabled={!bindings || saving} onClick={() => void save(name, true)} aria-label={`恢复${label}的默认快捷键`}>恢复默认</Button>}
      </dd>
    </div>)}</dl>
    <Text as="p" size="1" color="gray" className="options-help">默认快捷键沿用旧插件。{editable ? '点击“修改”后按下新的组合，保存后生效。' : '点击下方按钮，在浏览器中自定义快捷键。'}</Text>
    {editing && <Text as="p" size="1" color="gray" id="shortcut-recording-help" className="shortcut-recording-help">按 Escape 取消，按 Backspace 或 Delete 清除。</Text>}
    {(!firefox || api?.openShortcutSettings) && <div className="options-card-actions"><Button variant="soft" size="1" color="gray" disabled={demo} onClick={() => void openSettings()}><ExternalLink size={14}/>自定义快捷键</Button></div>}
    {demo && <Text as="p" size="1" color="gray" className="shortcut-recording-help">预览展示默认组合，安装扩展后可修改。</Text>}
    {error && <Callout.Root mt="3" size="1" color="amber" role="alert"><Callout.Text>{error}</Callout.Text></Callout.Root>}
    {notice && <Text as="p" size="1" color="gray" className="shortcut-recording-help" role="status">{notice}</Text>}
  </>;
}
