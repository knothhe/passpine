import { useCopy } from './LanguageProvider';
import { useState } from 'react';
import { captureShortcut } from '../lib/shortcuts';
import { isMac } from '../lib/popup-shortcuts';

export function ShortcutKeys({ value }: { value: string }) {
  const copy = useCopy();
  const names: Record<string, string> = { ...copy.shortcuts.keyNames, ...copy.shortcuts.modifierNames(isMac()) };
  const parts = value.replace(/([⌘⌃⌥⇧])/g, '$1+').split('+').filter(Boolean);
  return <>{parts.map((key, index) => <kbd key={index}>{names[key] ?? key}</kbd>)}</>;
}

export function ShortcutRecorder({ label, value, disabled, command = false, change, invalid }: { label: string; value: string; disabled?: boolean; command?: boolean; change(value: string): void; invalid(): void }) {
  const copy = useCopy();
  const [recording, setRecording] = useState(false);
  return <button type="button" className="shortcut-recorder" aria-label={copy.shortcuts.modify(label)} aria-pressed={recording} data-recording={recording} disabled={disabled} onClick={() => setRecording(true)} onBlur={() => setRecording(false)} onKeyDown={event => {
    if (!recording) return;
    if (event.key === 'Tab') { setRecording(false); return; }
    event.preventDefault(); event.stopPropagation();
    const result = captureShortcut(event.nativeEvent, isMac(), !command);
    if (result.type === 'cancel') setRecording(false);
    else if (result.type === 'shortcut') { setRecording(false); change(result.shortcut); }
    else if (result.type === 'invalid') invalid();
  }}>{recording ? <span className="shortcut-placeholder" role="status">{copy.shortcuts.recording}</span> : value ? <ShortcutKeys value={value}/> : <span className="shortcut-placeholder">{copy.shortcuts.record}</span>}</button>;
}
