import { useCopy, useLanguage } from './LanguageProvider';
import { useCallback, useEffect, useRef, useState } from 'react';
import { Button, Callout, Checkbox, IconButton, Slider, Text, TextField } from '@radix-ui/themes';
import { Check, Copy, RefreshCw } from 'lucide-react';
import { errorMessage } from '../lib/errors';
import type { Client, GeneratorOptions } from '../lib/types';
import { PageHeader } from './Primitives';
import './generator.css';
import { useExpiringValue } from '../lib/use-expiring-value';

export const initialGenerator: GeneratorOptions = { length: 24, lowercase: true, uppercase: true, numbers: true, symbols: true, excludeAmbiguous: true };

const groups = ['lowercase', 'uppercase', 'numbers', 'symbols'] as const;
const optionsKey = (options: GeneratorOptions) => `${options.length}:${+options.lowercase}${+options.uppercase}${+options.numbers}${+options.symbols}${+options.excludeAmbiguous}`;
const normalizeLength = (length: number) => Math.max(8, Math.min(128, Math.round(length)));

export function Generator({ client, open, close, usePassword, canSave, useLabel, backLabel }: { client: Client; open: boolean; close(): void; usePassword(value: string): void; canSave: boolean; useLabel?: string; backLabel?: string }) {
  const copy = useCopy();
  const { locale } = useLanguage();
  const [options, setOptions] = useState(initialGenerator);
  const [lengthDraft, setLengthDraft] = useState(String(initialGenerator.length));
  const [result, setResult] = useState<{ password: string; key: string; request: number }>();
  const [busy, setBusy] = useState(false), [error, setError] = useState('');
  const [copied, setCopied] = useExpiringValue(false, 2000);
  useEffect(() => { setError(''); }, [locale]);
  const generation = useRef(0), visible = useRef(open), latestKey = useRef(optionsKey(options));
  const screen = useRef<HTMLElement>(null);
  const key = optionsKey(options);
  visible.current = open;
  latestKey.current = key;
  const generate = useCallback(async () => {
    if (!visible.current) return;
    const request = ++generation.current, requestedKey = optionsKey(options);
    setBusy(true); setError(''); setCopied(false);
    try {
      const next = await client.request<{ password: string }>({ action: 'generate', options });
      if (!visible.current || request !== generation.current || requestedKey !== latestKey.current) return;
      if (!next.password) throw new Error(copy.generator.missingPassword);
      setResult({ password: next.password, key: requestedKey, request });
    } catch (failure) {
      if (visible.current && request === generation.current && requestedKey === latestKey.current) {
        setResult(undefined); setError(errorMessage(failure, copy));
      }
    } finally {
      if (visible.current && request === generation.current && requestedKey === latestKey.current) setBusy(false);
    }
  }, [client, options]);
  useEffect(() => {
    visible.current = open;
    if (open) void generate();
    else { generation.current++; setResult(undefined); setBusy(false); setError(''); setCopied(false); setLengthDraft(String(options.length)); }
    return () => { generation.current++; visible.current = false; };
  }, [open, generate]);
  useEffect(() => { if (open) screen.current?.querySelector<HTMLHeadingElement>('h1')?.focus({ preventScroll: true }); }, [open]);
  const updateLength = (length: number) => {
    const next = normalizeLength(length);
    setLengthDraft(String(next));
    setOptions(previous => previous.length === next ? previous : { ...previous, length: next });
  };
  const commitLength = () => updateLength(lengthDraft.trim() && Number.isFinite(Number(lengthDraft)) ? Number(lengthDraft) : options.length);
  const lengthValid = lengthDraft.trim() !== '' && Number.isInteger(Number(lengthDraft)) && Number(lengthDraft) === options.length;
  const ready = open && !busy && lengthValid && !!result?.password && result.key === key && result.request === generation.current;
  const leave = () => { visible.current = false; generation.current++; setResult(undefined); setBusy(false); close(); };
  const copyPassword = async () => {
    if (!ready || !result) return;
    const current = result.request;
    setError(''); setCopied(false);
    try {
      await client.copy(result.password);
      if (visible.current && current === generation.current && result.key === latestKey.current) setCopied(true);
    } catch (failure) {
      if (visible.current && current === generation.current && result.key === latestKey.current) setError(errorMessage(failure, copy));
    }
  };
  const selectedGroups = groups.filter(group => options[group]).length;
  if (!open) return null;
  return <section ref={screen} className="popup-screen generator-screen">
    <PageHeader title={copy.generator.title} back={leave} backLabel={backLabel}/>
    <main className="screen-content generator-content">
      <div className="generator-preview" data-busy={busy || undefined}>
        <TextField.Root readOnly aria-label={copy.generator.generatedPassword} value={result?.password ?? ''} placeholder={busy ? copy.generator.generating : copy.generator.generatePrompt} className="generator-password"/>
        <IconButton type="button" aria-label={copy.generator.regenerate} aria-busy={busy || undefined} title={copy.generator.regeneratePassword} variant="ghost" color="gray" className="generator-refresh" onClick={() => void generate()}><RefreshCw size={16}/></IconButton>
      </div>
      <div className="generator-length-row">
        <Text as="label" htmlFor="password-length-number" size="1" color="gray">{copy.generator.length}</Text>
        <Slider aria-label={copy.generator.passwordLength} min={8} max={128} step={1} value={[options.length]} onValueChange={value => updateLength(value[0] ?? options.length)}/>
        <TextField.Root id="password-length-number" aria-label={copy.generator.lengthValue} type="number" min={8} max={128} step={1} value={lengthDraft} className="generator-length-number" onChange={event => {
          const value = event.target.value, length = Number(value);
          setLengthDraft(value); setCopied(false);
          if (value.trim() && Number.isInteger(length) && length >= 8 && length <= 128) setOptions(previous => previous.length === length ? previous : { ...previous, length });
        }} onBlur={commitLength} onKeyDown={event => { if (event.key === 'Enter') { event.preventDefault(); commitLength(); } }}/>
      </div>
      <div className="generator-character-groups" role="group" aria-label={copy.generator.characterGroups}>{groups.map(group => <Button key={group} type="button" aria-label={copy.generator.groups[group].label} title={copy.generator.groups[group].label} aria-pressed={options[group]} disabled={options[group] && selectedGroups === 1} variant="outline" color="gray" className="generator-character-toggle" onClick={() => setOptions(previous => previous[group] && groups.every(candidate => candidate === group || !previous[candidate]) ? previous : { ...previous, [group]: !previous[group] })}>{copy.generator.groups[group].sample}</Button>)}</div>
      <label className="generator-exclusion-row"><Checkbox checked={options.excludeAmbiguous} onCheckedChange={checked => setOptions(previous => ({ ...previous, excludeAmbiguous: checked === true }))}/><Text size="1">{copy.generator.excludeAmbiguous}</Text><Text size="1" color="gray" className="generator-ambiguous-sample" aria-hidden="true">{copy.generator.ambiguousSample}</Text></label>
    </main>
    {error && <Callout.Root color="red" size="1" role="alert" className="screen-error"><Callout.Text>{error}</Callout.Text></Callout.Root>}
    <footer className="screen-footer generator-actions"><Button type="button" color="gray" variant="soft" disabled={!ready} onClick={() => void copyPassword()}>{copied ? <Check size={14}/> : <Copy size={14}/>} {copied ? copy.common.copied : copy.common.copy}</Button><Button type="button" className="grow" disabled={!ready || !canSave} onClick={() => { if (ready && result) usePassword(result.password); }}>{useLabel ?? copy.generator.useForNewAccount}</Button></footer>
  </section>;
}
