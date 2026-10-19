import { useCopy, useLanguage } from './LanguageProvider';
import { useEffect, useRef, useState } from 'react';
import { Badge, Button, Callout, Card, Heading, Text } from '@radix-ui/themes';
import { Check, ClipboardCopy, Globe2, RefreshCw } from 'lucide-react';
import { Brand } from './Primitives';
import { ShortcutSettings } from './ShortcutSettings';
import { AppearanceSettings } from './AppearanceSettings';
import { LanguageSettings } from './LanguageSettings';
import { About } from './About';
import { errorMessage } from '../lib/errors';
import { HOST, type Client, type Snapshot } from '../lib/types';
import { useExpiringValue } from '../lib/use-expiring-value';

export function Options({ client, extensionId, firefox = false }: { client: Client; extensionId: string; firefox?: boolean }) {
  const copy = useCopy();
  const { locale } = useLanguage();
  const [page, setPage] = useState<'settings' | 'about'>(() => new URLSearchParams(location.search).get('section') === 'about' ? 'about' : 'settings');
  const contentRef = useRef<HTMLDivElement>(null), previousPage = useRef(page);
  useEffect(() => { document.title = page === 'about' ? copy.about.pageTitle : copy.options.pageTitle; }, [page, locale, copy]);
  useEffect(() => {
    if (previousPage.current !== page) contentRef.current?.querySelector('h1')?.focus();
    previousPage.current = page;
  }, [page]);
  const [snapshot, setSnapshot] = useState<Snapshot>();
  const [error, setError] = useState('');
  const [checking, setChecking] = useState(true);
  const [copied, setCopied] = useExpiringValue(false, 2000);
  const [notice, setNotice] = useExpiringValue('', 4500);
  useEffect(() => { setError(''); setNotice(''); }, [locale]);
  const refreshSequence = useRef(0);
  const refresh = async (manual = false) => {
    const sequence = ++refreshSequence.current;
    setChecking(true); if (manual) setNotice('');
    try {
      const state = await client.request<Snapshot>({ action: 'snapshot' });
      if (sequence !== refreshSequence.current) return;
      setSnapshot(state); setError(''); if (manual) setNotice(copy.options.connectionChecked);
    } catch (error) {
      if (sequence !== refreshSequence.current) return;
      setSnapshot(undefined); setError(errorMessage(error, copy));
    } finally {
      if (sequence === refreshSequence.current) setChecking(false);
    }
  };
  useEffect(() => { void refresh(); return () => { refreshSequence.current++; }; }, [client]);
  const copyId = async () => {
    setCopied(false);
    try { await client.copy(extensionId); setCopied(true); }
    catch (error) { setError(errorMessage(error, copy)); }
  };
  return <main className="options-page">
    <header className="options-header"><Brand/>{client.demo && <Badge color="gray">{copy.preview.exampleData}</Badge>}<nav className="options-navigation" aria-label={copy.about.navigation}>
      <button type="button" aria-current={page === 'settings' ? 'page' : undefined} onClick={() => setPage('settings')}>{copy.about.settings}</button>
      <button type="button" aria-current={page === 'about' ? 'page' : undefined} onClick={() => setPage('about')}>{copy.about.title}</button>
    </nav></header>
    <div className="options-content" ref={contentRef}>{page === 'about' ? <About extensionId={extensionId} firefox={firefox}/> : <><div className="options-title"><Heading as="h1" size="6" tabIndex={-1}>{copy.options.title}</Heading><Text as="p" size="2" color="gray">{copy.options.description}</Text></div>
    <div className="options-sections">
      <Card size="3" className="options-card"><LanguageSettings/></Card>
      <Card size="3" className="options-card"><AppearanceSettings/></Card>
      <Card size="3" className="options-card options-card-connection">
        <div className="options-connection">
          <div className="options-connection-summary"><span className={`connection-dot ${!snapshot || snapshot.locked ? 'locked' : ''}`} aria-hidden="true"/><div><Heading as="h2" size="3">{checking ? copy.options.checkingConnection : snapshot ? snapshot.locked ? copy.options.connectedLocked : copy.connection.status.connected : copy.connection.disconnectedTitle}</Heading><Text as="p" size="2" color="gray">{snapshot?.vaultName || (snapshot ? copy.options.openVault : copy.options.connectLocal)}</Text></div></div>
          <Button variant="soft" aria-busy={checking || undefined} disabled={checking} onClick={() => void refresh(true)}><RefreshCw size={15} className={checking ? 'connection-checking' : undefined}/>{checking ? copy.options.checking : copy.options.checkConnection}</Button>
        </div>
        {error && <Callout.Root mt="4" size="1" color="amber" role="alert"><Callout.Text>{error}</Callout.Text></Callout.Root>}
        {notice && <Text as="p" size="1" color="gray" mt="3" role="status">{notice}</Text>}
        {!snapshot && !checking && <ol className="setup-steps">{copy.options.setupSteps.map((step, index) => <SetupStep key={index} number={String(index + 1)}>{step}</SetupStep>)}</ol>}
      </Card>
      <Card size="3" className="options-card">
        <ShortcutSettings firefox={firefox} demo={client.demo}/>
      </Card>
      <Card size="3" className="options-card">
        <div className="options-section-heading"><Globe2 size={18}/><Heading as="h2" size="3">{copy.options.connectionInfo}</Heading></div>
        <dl className="diagnostic-list"><div><dt>{copy.options.nativeHost}</dt><dd><code>{HOST}</code></dd></div><div><dt>{copy.options.extensionID}</dt><dd><code>{extensionId}</code></dd></div></dl>
        <div className="options-card-actions"><Button variant="soft" color="gray" size="1" onClick={() => void copyId()}>{copied ? <Check size={14}/> : <ClipboardCopy size={14}/>} {copied ? copy.common.copied : copy.options.copyExtensionID}</Button></div>
      </Card>
    </div></>}
    </div>
  </main>;
}

function SetupStep({ number, children }: { number: string; children: React.ReactNode }) {
  return <li className="setup-step"><span aria-hidden="true">{number}</span><Text size="2">{children}</Text></li>;
}
