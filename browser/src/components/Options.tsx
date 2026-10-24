import { useCopy, useLanguage } from './LanguageProvider';
import { useEffect, useMemo, useRef, useState } from 'react';
import { Badge, Button, Callout, Heading, Text } from '@radix-ui/themes';
import { Check, ClipboardCopy, Command, Info, Monitor, Palette, RefreshCw } from 'lucide-react';
import { Brand } from './Primitives';
import { ShortcutSettings } from './ShortcutSettings';
import { AppearanceSettings } from './AppearanceSettings';
import { LanguageSettings } from './LanguageSettings';
import { ClientTargetSettings, useClientTarget } from './ClientTargetSettings';
import { About } from './About';
import { errorMessage } from '../lib/errors';
import { type Client, type Snapshot } from '../lib/types';
import { hostForTarget } from '../lib/client-target';
import { useExpiringValue } from '../lib/use-expiring-value';
import { SettingsGroup, SettingsRow } from './SettingsLayout';
import { version } from '../../package.json';
import './options.css';

type SettingsPage = 'connection' | 'appearance' | 'shortcuts' | 'about';
const settingsPages: SettingsPage[] = ['connection', 'appearance', 'shortcuts', 'about'];
function pageFromLocation(): SettingsPage {
  const hash = location.hash.slice(1);
  return settingsPages.includes(hash as SettingsPage) ? hash as SettingsPage : new URLSearchParams(location.search).get('section') === 'about' ? 'about' : 'connection';
}

export function Options({ client: sourceClient, extensionId, firefox = false }: { client: Client; extensionId: string; firefox?: boolean }) {
  const copy = useCopy();
  const { locale } = useLanguage();
  const clientTarget = useClientTarget(sourceClient.demo);
  const client = useMemo(() => sourceClient.scope?.() ?? sourceClient, [sourceClient, clientTarget.target, clientTarget.connectionRevision]);
  const [page, setPage] = useState<SettingsPage>(pageFromLocation);
  const [visited, setVisited] = useState<Set<SettingsPage>>(() => new Set([page]));
  const contentRef = useRef<HTMLDivElement>(null), previousPage = useRef(page);
  const titles = { connection: copy.options.connection, appearance: copy.options.preferences, shortcuts: copy.shortcuts.title, about: copy.about.title };
  useEffect(() => { document.title = copy.options.sectionPageTitle(titles[page]); }, [page, locale, copy]);
  useEffect(() => {
    const update = () => { const next = pageFromLocation(); setPage(next); setVisited(current => new Set([...current, next])); };
    window.addEventListener('hashchange', update); window.addEventListener('popstate', update);
    return () => { window.removeEventListener('hashchange', update); window.removeEventListener('popstate', update); };
  }, []);
  useEffect(() => {
    if (previousPage.current !== page) {
      contentRef.current?.querySelector<HTMLElement>(`#settings-${page} h1`)?.focus({ preventScroll: true });
      window.scrollTo?.({ top: 0 });
    }
    previousPage.current = page;
  }, [page]);
  const navigate = (next: SettingsPage) => {
    if (next === page) return;
    history.pushState(null, '', `#${next}`);
    setVisited(current => new Set([...current, next])); setPage(next);
  };
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
  useEffect(() => {
    if (!clientTarget.loading) { setSnapshot(undefined); setNotice(''); void refresh(); }
    return () => { refreshSequence.current++; };
  }, [client, clientTarget.loading, clientTarget.target]);
  const copyId = async () => {
    setCopied(false);
    try { await navigator.clipboard.writeText(extensionId); setCopied(true); }
    catch (error) { setError(errorMessage(error, copy)); }
  };
  const icons = { connection: Monitor, appearance: Palette, shortcuts: Command, about: Info };
  return <div className="options-page options-shell">
    <aside className="options-sidebar">
      <div className="options-brand"><Brand/></div>
      <nav className="options-navigation" aria-label={copy.about.navigation}>
        {settingsPages.map(id => { const Icon = icons[id]; return <a key={id} href={`#${id}`} aria-current={page === id ? 'page' : undefined} onClick={event => {
          if (event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return;
          event.preventDefault(); navigate(id);
        }}><Icon size={16} strokeWidth={1.6} aria-hidden="true"/><span>{titles[id]}</span></a>; })}
      </nav>
      <div className="options-sidebar-footer"><span>{copy.about.version(version)}</span>{client.demo && <Badge color="gray">{copy.preview.exampleData}</Badge>}</div>
    </aside>
    <main className="options-content" ref={contentRef}>
      <section id="settings-connection" hidden={page !== 'connection'} aria-labelledby="connection-heading">
        <div className="options-title"><Heading id="connection-heading" as="h1" size="6" tabIndex={-1}>{copy.options.connection}</Heading></div>
        <div className="settings-stack">
          <SettingsGroup caption={copy.options.connectionStatus}>
            <div className="settings-row"><div className="options-connection">
              <div className="options-connection-summary"><span className={`connection-dot ${checking ? 'disconnected' : !snapshot ? 'disconnected' : snapshot.locked ? 'locked' : ''}`} aria-hidden="true"/><div><Heading as="h2" size="3">{checking ? copy.options.checkingConnection : snapshot ? snapshot.locked ? copy.options.connectedLocked : copy.connection.status.connected : copy.connection.disconnectedTitle}</Heading><Text as="p" size="2" color="gray">{snapshot?.vaultName || (snapshot ? copy.options.openVault : copy.options.connectLocal)}</Text></div></div>
              <Button variant="outline" color="gray" aria-busy={checking || undefined} disabled={checking} onClick={() => void refresh(true)}><RefreshCw size={14} className={checking ? 'connection-checking' : undefined}/>{checking ? copy.options.checking : copy.options.checkConnection}</Button>
            </div>
            {error && <Callout.Root mt="4" size="1" color="amber" role="alert"><Callout.Text>{error}</Callout.Text></Callout.Root>}
            {notice && <Text as="p" size="1" color="gray" mt="3" role="status">{notice}</Text>}
            </div>
          </SettingsGroup>
          <ClientTargetSettings state={clientTarget}/>
          {!snapshot && !checking && <SettingsGroup caption={copy.options.setupTitle}><ol className="setup-steps">{copy.options.setupSteps.map((step, index) => <SetupStep key={index} number={String(index + 1)}>{step}</SetupStep>)}</ol></SettingsGroup>}
          <details className="settings-details" open><summary>{copy.options.diagnostics}</summary><SettingsGroup>
            <SettingsRow label={copy.options.nativeHost} control={<code className="settings-code">{hostForTarget(clientTarget.target)}</code>}/>
            <SettingsRow label={copy.options.extensionID} control={<code className="settings-code">{extensionId}</code>}/>
            <div className="settings-group-actions"><Button variant="ghost" color="gray" size="1" onClick={() => void copyId()}>{copied ? <Check size={14}/> : <ClipboardCopy size={14}/>} {copied ? copy.common.copied : copy.options.copyExtensionID}</Button></div>
          </SettingsGroup></details>
        </div>
      </section>
      {visited.has('appearance') && <section id="settings-appearance" hidden={page !== 'appearance'} aria-labelledby="appearance-heading">
        <div className="options-title"><Heading id="appearance-heading" as="h1" size="6" tabIndex={-1}>{copy.options.preferences}</Heading></div>
        <div className="settings-stack"><AppearanceSettings language={<LanguageSettings/>}/></div>
      </section>}
      {visited.has('shortcuts') && <section id="settings-shortcuts" hidden={page !== 'shortcuts'} aria-labelledby="shortcuts-heading">
        <div className="options-title"><Heading id="shortcuts-heading" as="h1" size="6" tabIndex={-1}>{copy.shortcuts.title}</Heading></div>
        <ShortcutSettings firefox={firefox} demo={client.demo}/>
      </section>}
      {visited.has('about') && <div id="settings-about" hidden={page !== 'about'}><About extensionId={extensionId} firefox={firefox}/></div>}
    </main>
  </div>;
}

function SetupStep({ number, children }: { number: string; children: React.ReactNode }) {
  return <li className="setup-step"><span aria-hidden="true">{number}</span><Text size="2">{children}</Text></li>;
}
