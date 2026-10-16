import { useEffect, useRef, useState } from 'react';
import { Badge, Button, Callout, Card, Heading, Text } from '@radix-ui/themes';
import { Check, ClipboardCopy, Globe2, RefreshCw } from 'lucide-react';
import { Brand } from './Primitives';
import { ShortcutSettings } from './ShortcutSettings';
import { AppearanceSettings } from './AppearanceSettings';
import { errorMessage } from '../lib/errors';
import { HOST, type Client, type Snapshot } from '../lib/types';
import { useExpiringValue } from '../lib/use-expiring-value';

export function Options({ client, extensionId, firefox = false }: { client: Client; extensionId: string; firefox?: boolean }) {
  const [snapshot, setSnapshot] = useState<Snapshot>();
  const [error, setError] = useState('');
  const [checking, setChecking] = useState(true);
  const [copied, setCopied] = useExpiringValue(false, 2000);
  const [notice, setNotice] = useExpiringValue('', 4500);
  const refreshSequence = useRef(0);
  const refresh = async (manual = false) => {
    const sequence = ++refreshSequence.current;
    setChecking(true); if (manual) setNotice('');
    try {
      const state = await client.request<Snapshot>({ action: 'snapshot' });
      if (sequence !== refreshSequence.current) return;
      setSnapshot(state); setError(''); if (manual) setNotice('连接检查完成');
    } catch (error) {
      if (sequence !== refreshSequence.current) return;
      setSnapshot(undefined); setError(errorMessage(error));
    } finally {
      if (sequence === refreshSequence.current) setChecking(false);
    }
  };
  useEffect(() => { void refresh(); return () => { refreshSequence.current++; }; }, [client]);
  const copyId = async () => {
    setCopied(false);
    try { await client.copy(extensionId); setCopied(true); }
    catch (error) { setError(errorMessage(error)); }
  };
  return <main className="options-page">
    <header className="options-header"><Brand/>{client.demo && <Badge color="gray">示例数据</Badge>}</header>
    <div className="options-content"><div className="options-title"><Heading as="h1" size="6">浏览器设置</Heading><Text as="p" size="2" color="gray">管理外观、客户端连接和浏览器快捷键。</Text></div>
    <div className="options-sections">
      <Card size="3" className="options-card"><AppearanceSettings/></Card>
      <Card size="3" className="options-card options-card-connection">
        <div className="options-connection">
          <div className="options-connection-summary"><span className={`connection-dot ${!snapshot || snapshot.locked ? 'locked' : ''}`} aria-hidden="true"/><div><Heading as="h2" size="3">{checking ? '正在检查连接…' : snapshot ? snapshot.locked ? '已连接 · 已锁定' : '已连接' : '未连接客户端'}</Heading><Text as="p" size="2" color="gray">{snapshot?.vaultName || (snapshot ? '请在客户端打开密码库' : '连接本机 Passpine')}</Text></div></div>
          <Button variant="soft" aria-busy={checking || undefined} disabled={checking} onClick={() => void refresh(true)}><RefreshCw size={15} className={checking ? 'connection-checking' : undefined}/>{checking ? '检查中' : '检查连接'}</Button>
        </div>
        {error && <Callout.Root mt="4" size="1" color="amber" role="alert"><Callout.Text>{error}</Callout.Text></Callout.Root>}
        {notice && <Text as="p" size="1" color="gray" mt="3" role="status">{notice}</Text>}
        {!snapshot && !checking && <ol className="setup-steps"><SetupStep number="1">打开客户端，创建或导入密码库。</SetupStep><SetupStep number="2">打开客户端设置 → 浏览器，点击“注册浏览器连接”。</SetupStep><SetupStep number="3">点击浏览器工具栏中的 Passpine。</SetupStep></ol>}
      </Card>
      <Card size="3" className="options-card">
        <ShortcutSettings firefox={firefox} demo={client.demo}/>
      </Card>
      <Card size="3" className="options-card">
        <div className="options-section-heading"><Globe2 size={18}/><Heading as="h2" size="3">连接信息</Heading></div>
        <dl className="diagnostic-list"><div><dt>Native Host</dt><dd><code>{HOST}</code></dd></div><div><dt>扩展 ID</dt><dd><code>{extensionId}</code></dd></div></dl>
        <div className="options-card-actions"><Button variant="soft" color="gray" size="1" onClick={() => void copyId()}>{copied ? <Check size={14}/> : <ClipboardCopy size={14}/>} {copied ? '已复制' : '复制扩展 ID'}</Button></div>
      </Card>
    </div>
    </div>
  </main>;
}

function SetupStep({ number, children }: { number: string; children: React.ReactNode }) {
  return <li className="setup-step"><span aria-hidden="true">{number}</span><Text size="2">{children}</Text></li>;
}
