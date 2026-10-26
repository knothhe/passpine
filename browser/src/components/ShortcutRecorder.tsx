import { useCopy } from './LanguageProvider';
import { useEffect, useId, useRef, useState } from 'react';
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
  const buttonRef = useRef<HTMLButtonElement>(null), descriptionId = useId();
  // Readomi's capture lifecycle also releases the keyboard when a retained
  // settings section is hidden. Keep Passpine's browser-compatible key parser.
  useEffect(() => {
    if (!recording) return;
    const cancel = () => setRecording(false);
    const keydown = (event: KeyboardEvent) => {
      if (disabled || buttonRef.current?.closest('[hidden]')) { cancel(); return; }
      if (event.key === 'Tab') { cancel(); return; }
      event.preventDefault(); event.stopPropagation();
      const result = captureShortcut(event, isMac(), !command);
      if (result.type === 'cancel') cancel();
      else if (result.type === 'shortcut') { cancel(); change(result.shortcut); }
      else if (result.type === 'invalid') invalid();
    };
    const observer = new MutationObserver(() => {
      if (buttonRef.current?.disabled || buttonRef.current?.closest('[hidden]')) cancel();
    });
    let ancestor: HTMLElement | null = buttonRef.current;
    while (ancestor) { observer.observe(ancestor, { attributes: true, attributeFilter: ['hidden', 'disabled'] }); ancestor = ancestor.parentElement; }
    document.addEventListener('keydown', keydown, true);
    return () => { observer.disconnect(); document.removeEventListener('keydown', keydown, true); };
  }, [recording, disabled, command, change, invalid]);
  return <button ref={buttonRef} type="button" className="shortcut-recorder" aria-label={copy.shortcuts.modify(label)} aria-describedby={descriptionId} aria-pressed={recording} data-recording={recording} disabled={disabled} onClick={() => { buttonRef.current?.focus(); setRecording(true); }} onBlur={() => setRecording(false)}>
    <span id={descriptionId} className="settings-sr-only" aria-live="polite">{recording ? copy.shortcuts.recording : value || copy.common.notSet}</span>
    <span className="shortcut-keycaps" aria-hidden="true">{recording ? <span className="shortcut-placeholder">{copy.shortcuts.recording}</span> : value ? <ShortcutKeys value={value}/> : <span className="shortcut-placeholder">{copy.common.notSet}</span>}</span>
  </button>;
}
