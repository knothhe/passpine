import { useCallback, useEffect, useRef, useState } from 'react';
import { Badge, Button, Callout, Checkbox, IconButton, Slider, Text } from '@radix-ui/themes';
import { Check, Copy, Plus, RefreshCw } from 'lucide-react';
import { errorMessage } from '../lib/errors';
import type { Client, GeneratorOptions } from '../lib/types';
import { PageHeader } from './Primitives';

export const initialGenerator: GeneratorOptions = { length: 24, lowercase: true, uppercase: true, numbers: true, symbols: true, excludeAmbiguous: true };

export function Generator({ client, open, close, usePassword, canSave }: { client: Client; open: boolean; close(): void; usePassword(value: string): void; canSave: boolean }) {
  const [options, setOptions] = useState(initialGenerator), [password, setPassword] = useState(''), [error, setError] = useState(''), [copied, setCopied] = useState(false);
  const generation = useRef(0), visible = useRef(open);
  visible.current = open;
  const generate = useCallback(async () => {
    const request = ++generation.current;
    setError(''); setCopied(false);
    try { const result = await client.request<{ password: string }>({ action: 'generate', options }); if (visible.current && request === generation.current) setPassword(result.password); }
    catch (error) { if (visible.current && request === generation.current) setError(errorMessage(error)); }
  }, [client, options]);
  useEffect(() => {
    visible.current = open;
    if (open) void generate(); else { generation.current++; setPassword(''); }
    return () => { generation.current++; visible.current = false; };
  }, [open, generate]);
  const groups = [['lowercase', '小写字母', 'a–z'], ['uppercase', '大写字母', 'A–Z'], ['numbers', '数字', '0–9'], ['symbols', '符号', '!@#']] as const;
  if (!open) return null;
  return <section className="popup-screen generator-screen">
    <PageHeader title="密码生成器" back={close}/>
    <main className="screen-content generator-content">
      <div className="generated-password"><Text size="4" className="secret-text">{password || '••••••••••••••••'}</Text><IconButton aria-label="重新生成" variant="ghost" color="gray" onClick={() => void generate()}><RefreshCw size={17}/></IconButton></div>
      <div className="generator-length"><Text as="label" htmlFor="password-length" size="2" weight="medium">密码长度</Text><Badge variant="soft">{options.length} 位</Badge></div>
      <Slider id="password-length" aria-label="密码长度" min={8} max={64} step={1} value={[options.length]} onValueChange={value => setOptions({ ...options, length: value[0] ?? 24 })}/>
      <div className="generator-options">{groups.map(([key, label, hint]) => <label key={key}><Checkbox checked={options[key]} onCheckedChange={checked => setOptions({ ...options, [key]: checked === true })}/><Text size="2">{label}</Text><Text size="1" color="gray">{hint}</Text></label>)}</div>
      <label className="checkbox-label"><Checkbox checked={options.excludeAmbiguous} onCheckedChange={checked => setOptions({ ...options, excludeAmbiguous: checked === true })}/><span><Text as="div" size="2">排除易混淆字符</Text><Text as="div" size="1" color="gray">I l 1 O 0</Text></span></label>
    </main>
    {error && <Callout.Root color="red" size="1" role="alert" className="screen-error"><Callout.Text>{error}</Callout.Text></Callout.Root>}
    <footer className="screen-footer"><Button color="gray" variant="soft" disabled={!password} onClick={() => { void client.copy(password).then(() => setCopied(true)).catch(error => setError(errorMessage(error))); }}>{copied ? <Check size={15}/> : <Copy size={15}/>} {copied ? '已复制' : '复制'}</Button><Button className="grow" disabled={!password || !canSave} onClick={() => usePassword(password)}><Plus size={15}/>用于新账号</Button></footer>
  </section>;
}
