import { useCopy, useLanguage } from './LanguageProvider';
import { useCallback, useEffect, useLayoutEffect, useRef, useState } from 'react';
import { AnimatePresence, motion, MotionConfig, useIsPresent, useReducedMotion } from 'motion/react';
import { AlertDialog, Badge, Button, Flex, Heading, IconButton, Separator, Spinner, Text, TextField } from '@radix-ui/themes';
import { ArrowDownToLine, Check, CircleAlert, Copy, Eye, EyeOff, Globe2, KeyRound, Link2, Link2Off, LockKeyhole, Pencil, Plus, RefreshCw, Search, Settings2, Trash2, X } from 'lucide-react';
import { entriesFor, parseDocument } from '../lib/model';
import { errorMessage } from '../lib/errors';
import type { Client, Entry, EntryDocument, FillResult, SessionStatus, Snapshot } from '../lib/types';
import { ActionIcon, ApplicationIcon, PageHeader, SecretField } from './Primitives';
import { ConnectionState } from './ConnectionState';
import { AccountList } from './AccountList';
import { EditorForm } from './EditorForm';
import { SessionGuard } from '../lib/session';
import { matchesPopupShortcut, usePopupShortcuts } from '../lib/popup-shortcuts';
import { ShortcutKeys } from './ShortcutRecorder';
import { useExpiringValue } from '../lib/use-expiring-value';
import { subscribeClientTarget } from '../lib/client-target';

export type Detail = EntryDocument & { entry: Entry; vaultId: string; receivedAt: number };
export type EditorValue = { vaultId: string; original?: Detail; password?: string };
type DetailTarget = { entry: Entry; vaultId: string };
type RefreshResult = { ok: boolean; error?: string };
type PopupView = 'list' | 'detail' | 'editor';

export function App({ client }: { client: Client }) {
  const [connectionGeneration, setConnectionGeneration] = useState(0);
  useEffect(() => subscribeClientTarget(() => {}, client.demo, () => setConnectionGeneration(value => value + 1)), [client.demo]);
  // A different client owns a different vault session, including when both
  // clients happen to return the same vault ID or metadata revision.
  return <VaultApp key={connectionGeneration} client={client}/>;
}

function VaultApp({ client: sourceClient }: { client: Client }) {
  const [client] = useState(() => sourceClient.scope?.() ?? sourceClient);
  const copy = useCopy();
  const { locale } = useLanguage();
  const [snapshot, setSnapshot] = useState<Snapshot>();
  const [connectionError, setConnectionError] = useState<unknown>();
  const [query, setQuery] = useState('');
  const searchInput = useRef<HTMLInputElement>(null);
  const clearSearch = () => { setQuery(''); searchInput.current?.focus({ preventScroll: true }); };
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const shortcuts = usePopupShortcuts(client.demo);
  const [busy, setBusy] = useState(false);
  const [notice, setNotice] = useExpiringValue<{ text: string; error?: boolean } | undefined>(undefined, 4500);
  useEffect(() => { setNotice(undefined); }, [locale]);
  const [detail, setDetail] = useState<Detail>();
  const [detailTarget, setDetailTarget] = useState<DetailTarget>();
  const [revealed, setRevealed] = useState(false);
  const [editor, setEditor] = useState<EditorValue>();
  const [deleteOpen, setDeleteOpen] = useState(false);

  const busyOperation = useRef(false);
  const operationRequest = useRef(0);
  const detailNavigation = useRef(0);
  const opening = useRef<{ id: number; vaultId: string; awaitingUnlock: boolean } | undefined>(undefined);
  const session = useRef(new SessionGuard());
  const latestSnapshot = useRef<Snapshot | undefined>(undefined);
  const detailRequest = useRef(0);
  const refreshPending = useRef<Promise<RefreshResult> | undefined>(undefined);
  const manualRefreshPending = useRef(false);
  const listScroll = useRef(0), returnEntry = useRef<string | undefined>(undefined);
  const [otpRemaining, setOtpRemaining] = useState(0);
  const cancelDetails = useCallback(() => {
    detailNavigation.current++;
    detailRequest.current++;
    setDetailTarget(undefined); setDetail(undefined); setRevealed(false);
    if (opening.current) {
      opening.current = undefined;
      operationRequest.current++;
      busyOperation.current = false;
      setBusy(false);
    }
  }, []);
  const clearSecrets = useCallback((preservePending = false) => {
    if (preservePending) { detailRequest.current++; setDetail(undefined); setRevealed(false); }
    else cancelDetails();
    setEditor(undefined); setDeleteOpen(false);
  }, [cancelDetails]);
  const disconnected = useCallback((error: unknown) => {
    session.current.disconnect(); latestSnapshot.current = undefined;
    setConnectionError(error); setSnapshot(undefined); clearSecrets();
  }, [clearSecrets]);
  const observe = useCallback((state: SessionStatus, sequence: number) => {
    const result = session.current.observe(state, sequence);
    // Expected authentication can preserve the non-secret loading shell. Locks,
    // vault switches and unrelated metadata changes still cancel the request.
    const expectedUnlock = !!opening.current?.awaitingUnlock && state.configured && !state.locked && state.vaultId === opening.current.vaultId;
    if (result.invalidated) clearSecrets(expectedUnlock);
    else if (result.metadataChanged) {
      if (expectedUnlock) { detailRequest.current++; setDetail(undefined); setRevealed(false); }
      else cancelDetails();
    }
    return result;
  }, [clearSecrets, cancelDetails]);
  const refresh = useCallback((): Promise<RefreshResult> => {
    if (refreshPending.current) return refreshPending.current;
    const task = (async () => {
      const sequence = session.current.probe();
      try {
        const next = await client.request<Snapshot>({ action: 'snapshot' });
        if (!observe(next, sequence).accepted) return { ok: false };
        latestSnapshot.current = next; setSnapshot(next); setConnectionError('');
        setDetail(previous => previous?.vaultId === next.vaultId ? { ...previous, entry: { ...previous.entry, matched: next.matches.includes(previous.entry.path) } } : previous);
        return { ok: true };
      } catch (error) { disconnected(error); return { ok: false, error: errorMessage(error, copy) }; }
      finally { setLoading(false); }
    })();
    refreshPending.current = task;
    void task.finally(() => { if (refreshPending.current === task) refreshPending.current = undefined; });
    return task;
  }, [client, observe, disconnected]);
  const refreshManually = async () => {
    if (manualRefreshPending.current) return;
    manualRefreshPending.current = true; setRefreshing(true); setNotice(undefined);
    try { const result = await refresh(); setNotice(result.ok ? { text: copy.vault.refreshed } : { text: result.error ?? copy.vault.refreshFailed, error: true }); }
    finally { manualRefreshPending.current = false; setRefreshing(false); }
  };
  const checkStatus = useCallback(async () => {
    const sequence = session.current.probe();
    const state = await client.request<SessionStatus>({ action: 'status' });
    const observation = observe(state, sequence);
    const previous = latestSnapshot.current;
    const needsSnapshot = !previous || previous.vaultId !== state.vaultId || previous.locked !== state.locked || previous.configured !== state.configured || previous.entriesVersion !== state.entriesVersion || previous.associationVersion !== state.associationVersion;
    if (observation.accepted && (observation.invalidated || observation.metadataChanged || needsSnapshot)) {
      setSnapshot(previous => previous && previous.vaultId === state.vaultId ? { ...previous, locked: state.locked } : undefined);
      void refresh();
    }
    return state;
  }, [client, observe, refresh]);
  useEffect(() => { void refresh(); }, [refresh]);
  useEffect(() => {
    let pending = false;
    const timer = setInterval(() => {
      if (busyOperation.current || pending || refreshPending.current || !latestSnapshot.current || window.document.hidden) return;
      pending = true;
      void checkStatus().catch(disconnected).finally(() => { pending = false; });
    }, 2000);
    const visibility = () => { if (window.document.hidden) clearSecrets(); else if (!busyOperation.current && latestSnapshot.current) void checkStatus().catch(disconnected); };
    window.document.addEventListener('visibilitychange', visibility);
    return () => { clearInterval(timer); window.document.removeEventListener('visibilitychange', visibility); session.current.disconnect(); };
  }, [checkStatus, disconnected, clearSecrets]);
  useEffect(() => { setRevealed(false); }, [detail?.entry.path]);
  useEffect(() => {
    if (!detail?.otp || !detail.otpSeconds || editor) { setOtpRemaining(0); return; }
    let pending = false, cancelled = false;
    const deadline = detail.receivedAt + detail.otpSeconds * 1000;
    const tick = () => {
      const remaining = Math.max(0, Math.ceil((deadline - Date.now()) / 1000));
      setOtpRemaining(remaining);
      if (remaining || pending) return;
      pending = true;
      const token = session.current.capture(), request = detailRequest.current;
      void client.request<EntryDocument>({ action: 'fetch', vaultId: detail.vaultId, entry: detail.entry.path }).then(async doc => {
        await checkStatus();
        if (!cancelled && request === detailRequest.current && session.current.valid(token)) setDetail({ ...doc, entry: detail.entry, vaultId: detail.vaultId, receivedAt: Date.now() });
      }).catch(error => { if (!cancelled) { cancelDetails(); setNotice({ text: errorMessage(error, copy), error: true }); } });
    };
    tick(); const timer = setInterval(tick, 250);
    return () => { cancelled = true; clearInterval(timer); };
  }, [client, detail, editor, checkStatus, cancelDetails]);
  const run = async (operation: () => Promise<void>) => {
    if (busyOperation.current) return;
    const request = ++operationRequest.current;
    busyOperation.current = true;
    setBusy(true); setNotice(undefined);
    try { await operation(); } catch (error) { if (request === operationRequest.current) setNotice({ text: errorMessage(error, copy), error: true }); }
    finally { if (request === operationRequest.current) { busyOperation.current = false; setBusy(false); } }
  };
  const unlock = async () => {
    await client.request({ action: 'unlock' });
    // A probe started before authentication may still contain the locked state.
    // Let it finish, then request a snapshot belonging to the new session.
    await refreshPending.current;
    await refresh();
  };
  const ensureUnlocked = async (vaultId: string) => {
    if (latestSnapshot.current?.locked) await unlock();
    const state = latestSnapshot.current;
    if (!state || state.vaultId !== vaultId) throw new Error(copy.vault.changed);
    if (state.locked) throw new Error(copy.vault.unlockRequired);
  };
  const copyEntry = (entry: Entry, field: 'password' | 'username') => void run(async () => {
    if (!snapshot) return;
    await ensureUnlocked(snapshot.vaultId);
    await client.request({ action: 'copy', vaultId: snapshot.vaultId, entry: entry.path, field });
    setNotice({ text: client.demo ? copy.preview.credentialsNotCopied : copy.common.copied });
  });
  const open = (entry: Entry) => void run(async () => {
    if (!snapshot) return;
    const navigation = ++detailNavigation.current, vaultId = snapshot.vaultId;
    listScroll.current = window.document.querySelector<HTMLElement>('.popup-page-list:not([aria-hidden="true"]) .entries-list')?.scrollTop ?? 0;
    returnEntry.current = entry.path;
    opening.current = { id: navigation, vaultId, awaitingUnlock: !!latestSnapshot.current?.locked };
    setDetailTarget({ entry, vaultId }); setDetail(undefined); setRevealed(false);
    try {
      await ensureUnlocked(vaultId);
      if (navigation !== detailNavigation.current || window.document.hidden) return;
      if (opening.current?.id === navigation) opening.current.awaitingUnlock = false;
      let token = session.current.capture(), request = ++detailRequest.current;
      const doc = await client.request<EntryDocument>({ action: 'fetch', vaultId, entry: entry.path, interactive: true });
      if (navigation !== detailNavigation.current) return;
      if (doc.session) {
        if (!doc.session.configured || doc.session.locked || doc.session.vaultId !== vaultId) throw new Error(copy.vault.sessionChanged);
        // The native response lease authenticates this fetched document. Adopt
        // its session before the follow-up probe, including an unlock that the
        // popup did not see between polls. Later session changes still cancel.
        if (opening.current?.id === navigation) opening.current.awaitingUnlock = true;
        observe(doc.session, session.current.probe());
        if (opening.current?.id === navigation) opening.current.awaitingUnlock = false;
        token = session.current.capture(); request = detailRequest.current;
      }
      await checkStatus();
      if (!session.current.valid(token) || request !== detailRequest.current || navigation !== detailNavigation.current || window.document.hidden) return;
      setDetail({ ...doc, entry, vaultId, receivedAt: Date.now() });
    } catch (error) {
      if (navigation !== detailNavigation.current) return;
      detailNavigation.current++; detailRequest.current++;
      setDetailTarget(undefined); setDetail(undefined); setRevealed(false);
      throw error;
    } finally { if (opening.current?.id === navigation) opening.current = undefined; }
  });
  const fill = (entry: Entry) => void run(async () => {
    if (!snapshot?.tab) return;
    const tab = snapshot.tab;
    await ensureUnlocked(snapshot.vaultId);
    const result = await client.request<FillResult>({ action: 'fill', vaultId: snapshot.vaultId, entry: entry.path, tab });
    if (!client.demo && result.filled > 0) { window.close(); return; }
    await refresh();
    const text = copy.vault.fillResult(result.filled, tab.hostname, result.associated);
    setNotice({ text: client.demo ? copy.preview.notice(text) : text, error: !result.associated });
  });
  const copyCredential = (field: 'password' | 'username' | 'otp') => void run(async () => {
    if (!detail) return;
    await client.request({ action: 'copy', vaultId: detail.vaultId, entry: detail.entry.path, field });
    setNotice({ text: client.demo ? copy.preview.credentialsNotCopied : copy.vault.copiedWithExpiry });
  });
  const lock = () => void run(async () => {
    clearSecrets(); session.current.disconnect();
    await client.request({ action: 'lock' }); await refresh();
  });
  const entries = snapshot ? entriesFor(snapshot, query) : [];
  const associate = (entry: Entry) => void run(async () => {
    if (!snapshot?.tab) return;
    await client.request({ action: entry.matched ? 'dissociate' : 'associate', vaultId: snapshot.vaultId, entry: entry.path, tab: snapshot.tab });
    await refresh(); setNotice({ text: (entry.matched ? copy.accounts.dissociatedWebsite : copy.accounts.associatedWebsite)(snapshot.tab.hostname) });
  });
  const newAccount = () => void run(async () => { if (!snapshot?.configured || editor) return; await ensureUnlocked(snapshot.vaultId); setEditor({ vaultId: snapshot.vaultId }); });
  useEffect(() => {
    const handle = (event: KeyboardEvent) => {
      if (editor || deleteOpen || window.document.hidden) return;
      const binding = shortcuts.find(item => matchesPopupShortcut(event, item.shortcut));
      if (!binding || busy && binding.action !== 'search') return;
      if (['copy', 'fill', 'edit'].includes(binding.action) && !detail) return;
      event.preventDefault();
      if (binding.action === 'search') { cancelDetails(); setTimeout(() => window.document.querySelector<HTMLInputElement>('.search-section input')?.focus(), 0); }
      else if (binding.action === 'new') newAccount();
      else if (binding.action === 'refresh') void refreshManually();
      else if (binding.action === 'settings') void client.openOptions();
      else if (binding.action === 'lock' && snapshot?.configured && !snapshot.locked) lock();
      else if (binding.action === 'copy') copyCredential('password');
      else if (binding.action === 'fill' && detail && snapshot?.tab) fill(detail.entry);
      else if (binding.action === 'edit' && detail && !busy) setEditor({ vaultId: detail.vaultId, original: detail });
    };
    window.addEventListener('keydown', handle); return () => window.removeEventListener('keydown', handle);
  });
  const document = detail ? parseDocument(detail.contents) : undefined;
  const detailEntry = detail?.entry ?? detailTarget?.entry;
  const page: PopupView = editor ? 'editor' : detailEntry ? 'detail' : 'list';
  const [navigation, setNavigation] = useState<{ page: PopupView; direction: number }>({ page, direction: 1 });
  if (navigation.page !== page) setNavigation({ page, direction: pageOrder[page] > pageOrder[navigation.page] ? 1 : -1 });
  const navigateEntries = (event: React.KeyboardEvent<HTMLDivElement>) => {
    if (page !== 'list' || busy || deleteOpen || window.document.hidden || event.defaultPrevented || event.nativeEvent.isComposing) return;
    const control = event.ctrlKey && !event.altKey && !event.metaKey && !event.shiftKey;
    const plain = !event.ctrlKey && !event.altKey && !event.metaKey && !event.shiftKey;
    const key = event.key.toLowerCase();
    const direction = control && key === 'n' || plain && key === 'arrowdown' ? 1 : control && key === 'p' || plain && key === 'arrowup' ? -1 : 0;
    if (!direction || !(event.target instanceof HTMLElement)) return;
    const list = event.target.closest<HTMLElement>('.popup-page-list:not([aria-hidden="true"])');
    if (!list) return;
    const search = list.querySelector<HTMLInputElement>('.search-section input');
    const row = event.target.closest<HTMLElement>('.entry-row');
    if (event.target !== search && (!row || event.target.closest('.account-more'))) return;
    const buttons = Array.from(list.querySelectorAll<HTMLButtonElement>('button[data-entry-path]:not(:disabled)'));
    const current = row?.querySelector<HTMLButtonElement>('button[data-entry-path]');
    const next = current ? buttons.indexOf(current) + direction : direction > 0 ? 0 : buttons.length - 1;
    event.preventDefault();
    (buttons[next] ?? search)?.focus();
  };

  return <MotionConfig reducedMotion="user" transition={{ duration: .16, ease: [.2, .8, .2, 1] }}><div className="popup-shell" onKeyDown={navigateEntries}>
    <div className="popup-pages"><AnimatePresence initial={false} mode="sync" custom={navigation.direction}><PopupPage key={page} view={page} direction={navigation.direction} listScroll={listScroll.current} returnEntry={returnEntry.current}>
    {editor ? <EditorForm client={client} value={editor} defaultURL={snapshot?.tab ? `https://${snapshot.tab.hostname}` : ''} close={() => setEditor(undefined)} saved={async () => { setEditor(undefined); cancelDetails(); await refresh(); setNotice({ text: copy.vault.encryptedSave }); }}/>
    : detailEntry ? <>
      <PageHeader title={copy.accounts.details} back={cancelDetails}><Button size="1" className="detail-fill-action" aria-label={copy.accounts.fillCredentials} title={snapshot?.tab ? copy.accounts.fillCredentials : copy.accounts.openWebsiteFirst} disabled={busy || !detail || !snapshot?.tab} onClick={() => detail && fill(detail.entry)}><ArrowDownToLine size={14}/><span>{copy.accounts.fillCredentials}</span></Button><ActionIcon label={copy.accounts.edit} disabled={!detail || busy} onClick={() => detail && setEditor({ vaultId: detail.vaultId, original: detail })}><Pencil size={16}/></ActionIcon><ActionIcon label={copy.accounts.delete} disabled={!detail || busy} onClick={() => setDeleteOpen(true)}><Trash2 size={16}/></ActionIcon></PageHeader>
      <main className="screen-content detail-content" aria-busy={!detail}>
        <div className="entry-summary"><div className="entry-text"><span className="entry-path" title={detailEntry.path}>{detailEntry.folder || copy.common.vault}</span><Heading as="h2" size="2" title={detailEntry.title}>{detailEntry.title}</Heading></div></div>
        <div className="detail-content-swap"><AnimatePresence initial={false}>{!detail && <motion.div className="detail-loading-layer" key="loading" exit={{ opacity: 0 }} transition={{ duration: .1 }}><DetailSkeleton/></motion.div>}</AnimatePresence>
        {detail && document && <motion.div className="detail-data-layer" initial={{ opacity: 0 }} animate={{ opacity: 1 }} transition={{ duration: .1 }}>
        <div className="credential-card">
          <SecretField label={copy.common.username} value={document.username || copy.common.notSet} onCopy={document.username ? () => copyCredential('username') : undefined}/>
          <Separator size="4"/>
          <div className="credential-field"><div><Text as="div" size="1" color="gray" className="credential-label">{copy.common.password}</Text><Text as="div" className="credential-value secret-text" size="2">{revealed ? document.password : copy.accounts.maskedPassword}</Text></div><Flex><ActionIcon label={revealed ? copy.common.hidePassword : copy.common.showPassword} onClick={() => setRevealed(!revealed)}>{revealed ? <EyeOff size={16}/> : <Eye size={16}/>}</ActionIcon><ActionIcon label={copy.common.copyPassword} onClick={() => copyCredential('password')}><Copy size={16}/></ActionIcon></Flex></div>
          {detail.otp && <><Separator size="4"/><SecretField label={copy.accounts.otp(otpRemaining)} value={otpRemaining > 0 ? detail.otp : copy.accounts.maskedOTP} onCopy={() => copyCredential('otp')}/></>}
          <Separator size="4"/>
          <div className="credential-field site-detail"><div><Text as="div" size="1" color="gray" className="credential-label">{copy.common.website}</Text><Text as="div" size="2" className="credential-value truncate" title={document.urls[0]}>{document.urls[0] || (detail.entry.matched ? snapshot?.tab?.hostname : copy.accounts.websiteNotSet)}</Text></div>{detail.entry.matched && <Check size={15} className="accent" aria-label={copy.accounts.websiteAssociated}/>}</div>
        </div>
        {snapshot?.tab && <Button className="detail-associate" variant="ghost" size="1" disabled={busy} title={(detail.entry.matched ? copy.accounts.dissociateWebsite : copy.accounts.associateWebsite)(snapshot.tab.hostname)} onClick={() => associate(detail.entry)}>{detail.entry.matched ? <Link2Off size={14}/> : <Link2 size={14}/>}<span className="truncate">{detail.entry.matched ? copy.accounts.dissociateCurrentWebsite : copy.accounts.associateWebsite(snapshot.tab.hostname)}</span></Button>}
        {document.notes && <div className="detail-notes"><Text as="div" size="1" color="gray" className="credential-label">{copy.common.notes}</Text><Text as="p" size="2" className="notes">{document.notes}</Text></div>}
        </motion.div>}</div>
      </main>
    </> : <>
      {snapshot?.configured && <div className="site-context"><Globe2 size={16}/><div className="site-context-label"><Text size="2" weight="medium" className="truncate" title={snapshot.tab?.hostname}>{snapshot.tab?.hostname || copy.accounts.websiteNotOpen}</Text><ActionIcon label={copy.accounts.create} onClick={newAccount} disabled={busy}><Plus size={16}/></ActionIcon></div>{snapshot.tab && !snapshot.tab.secure && <Badge variant="soft" color="amber" size="1">{copy.accounts.insecureProtocol}</Badge>}{snapshot.tab && <Text size="1" color="gray" className="site-match-count">{snapshot.matches.length > 0 ? copy.accounts.matchedCount(snapshot.matches.length) : copy.accounts.noMatches}</Text>}</div>}
      <div className="search-section"><TextField.Root ref={searchInput} aria-label={copy.accounts.searchVault} placeholder={copy.accounts.search} value={query} onChange={event => setQuery(event.target.value)} size="3" autoFocus><TextField.Slot><Search size={18}/></TextField.Slot><TextField.Slot>{query ? <IconButton aria-label={copy.accounts.clearSearch} size="1" variant="ghost" onClick={clearSearch}><X size={14}/></IconButton> : <span className="search-shortcut">{shortcuts.find(item => item.action === 'search')?.shortcut && <ShortcutKeys value={shortcuts.find(item => item.action === 'search')!.shortcut}/>}</span>}</TextField.Slot></TextField.Root></div>
      {loading ? <div className="empty-state"><Spinner size="3"/><Text size="2" color="gray">{copy.connection.connecting}</Text></div> : !snapshot ? <ConnectionState error={connectionError ? errorMessage(connectionError, copy) : ''} retry={() => void refreshManually()} settings={() => void client.openOptions()}/> : !snapshot.configured ? <div className="empty-state"><div className="empty-icon"><ApplicationIcon size={40}/></div><Heading size="4">{copy.vault.unconfigured}</Heading><Text size="2" color="gray">{copy.vault.setup}</Text><Button onClick={() => void refreshManually()}>{copy.connection.reconnect}</Button></div> : snapshot.locked && snapshot.entries.length === 0 ? <div className="empty-state"><div className="empty-icon"><LockKeyhole size={30}/></div><Heading size="4">{copy.vault.locked}</Heading><Text size="2" color="gray">{snapshot.vaultName}</Text><Button disabled={busy} onClick={() => void run(unlock)}><KeyRound size={16}/>{copy.vault.unlockInClient}</Button><Button variant="ghost" color="gray" onClick={() => void refreshManually()}>{copy.vault.refresh}</Button></div> : <>
        <div className="list-toolbar"><Text size="1" color="gray">{query ? copy.accounts.searchResults : copy.accounts.all}</Text><Text size="1" color="gray">{copy.accounts.count(entries.length)}</Text></div>
        <main className="entries-list">
          {entries.length > 0 && <AccountList entries={entries} onOpen={open} onFill={fill} onCopy={copyEntry} onAssociate={associate} website={snapshot.tab?.hostname} busy={busy}/>}
          {entries.length === 0 && <div className="no-results"><Search size={24}/><Text size="2">{query ? copy.accounts.noResults : copy.accounts.empty}</Text>{query && <><Text size="1" className="no-results-help">{copy.accounts.noResultsHelp}</Text><Button size="2" variant="soft" onClick={clearSearch}>{copy.accounts.clearSearch}</Button></>}</div>}
        </main>
      </>}
    </>}
    </PopupPage></AnimatePresence></div>
    {notice && <div className={`notice ${notice.error ? 'notice-error' : ''}`} role={notice.error ? 'alert' : 'status'}>{notice.error ? <CircleAlert size={16}/> : <Check size={16}/>}<span>{notice.text}</span><button aria-label={copy.vault.dismissNotice} onClick={() => setNotice(undefined)}><X size={14}/></button></div>}
    <footer className="app-footer">
      <div className="footer-connection" title={snapshot?.vaultName || copy.connection.clientConnection}><span className={`connection-dot ${!snapshot ? 'disconnected' : snapshot.locked ? 'locked' : ''}`} aria-hidden="true"/><span className="footer-vault">{snapshot?.vaultName || copy.connection.clientName}</span><span className="footer-state">{loading ? copy.connection.status.connecting : !snapshot ? copy.connection.status.disconnected : !snapshot.configured ? copy.common.notSet : snapshot.locked ? copy.connection.status.locked : copy.connection.status.connected}</span></div>
      <div className="footer-actions"><ActionIcon label={copy.vault.refreshVault} onClick={() => void refreshManually()} loading={loading || refreshing} disabled={loading || refreshing || busy}><RefreshCw size={15} className={loading || refreshing ? 'connection-checking' : undefined}/></ActionIcon>{snapshot?.configured && !snapshot.locked && <ActionIcon label={copy.vault.lock} onClick={lock} disabled={busy}><LockKeyhole size={16}/></ActionIcon>}<ActionIcon label={copy.vault.settings} onClick={() => void client.openOptions()}><Settings2 size={16}/></ActionIcon></div>
    </footer>
    {client.demo && <div className="demo-ribbon">{copy.preview.ribbon}</div>}

    <AlertDialog.Root open={deleteOpen} onOpenChange={setDeleteOpen}><AlertDialog.Content maxWidth="360px"><AlertDialog.Title>{copy.vault.trashTitle}</AlertDialog.Title><AlertDialog.Description size="2">{copy.vault.trashDescription(detail?.entry.title ?? '')}</AlertDialog.Description><Flex justify="end" gap="3" mt="5"><AlertDialog.Cancel><Button variant="soft" color="gray">{copy.common.cancel}</Button></AlertDialog.Cancel><Button color="red" disabled={busy} onClick={() => void run(async () => { if (!detail) return; await client.request({ action: 'delete', vaultId: detail.vaultId, entry: detail.entry.path, revision: detail.revision }); setDeleteOpen(false); cancelDetails(); await refresh(); setNotice({ text: copy.vault.movedToTrash }); })}><Trash2 size={15}/>{copy.vault.moveToTrash}</Button></Flex></AlertDialog.Content></AlertDialog.Root>
  </div></MotionConfig>;
}

const pageOrder = { list: 0, detail: 1, editor: 2 };
const pageVariants = {
  enter: (direction: number) => ({ opacity: 0, x: direction * 18 }),
  active: { opacity: 1, x: 0 },
  leave: (direction: number) => ({ opacity: 0, x: direction * -18 }),
};

function PopupPage({ view, direction, children, listScroll, returnEntry }: { view: PopupView; direction: number; children: React.ReactNode; listScroll: number; returnEntry?: string }) {
  const copy = useCopy();
  const present = useIsPresent();
  const reduceMotion = useReducedMotion();
  const ref = useRef<HTMLElement>(null);
  useLayoutEffect(() => {
    if (!present || window.document.hidden) return;
    const page = ref.current;
    if (view === 'detail') page?.querySelector<HTMLHeadingElement>('h1')?.focus({ preventScroll: true });
    else if (view === 'list' && returnEntry) {
      const list = page?.querySelector<HTMLElement>('.entries-list');
      if (list) list.scrollTop = listScroll;
      const entry = Array.from(page?.querySelectorAll<HTMLButtonElement>('button[data-entry-path]') ?? []).find(button => button.dataset.entryPath === returnEntry);
      (entry ?? page?.querySelector<HTMLInputElement>('.search-section input'))?.focus({ preventScroll: true });
    }
  }, [present, view]);
  return <motion.section ref={ref} className={`popup-page popup-page-${view}`} data-popup-page={view} data-navigation-direction={direction > 0 ? 'forward' : 'back'} inert={!present} aria-hidden={!present} custom={direction} variants={pageVariants} initial="enter" animate="active" exit="leave" transition={{ duration: reduceMotion ? 0 : .18, ease: [.2, .8, .2, 1] }}>
    {/* AnimatePresence retains removed children. Drop credentials immediately,
        retaining a neutral skeleton so the outgoing page does not flash blank. */}
    {present || view === 'list' ? children : <div className="page-transition-mask"><PageHeader title={view === 'detail' ? copy.accounts.details : copy.accounts.edit} back={() => {}} disabled/><div className="screen-content"><DetailSkeleton/></div></div>}
  </motion.section>;
}

function DetailSkeleton() {
  const copy = useCopy();
  return <div className="detail-skeleton">
    <div className="credential-card" aria-hidden="true"><div className="credential-field"><div className="grow"><Text as="div" size="1" color="gray" className="credential-label">{copy.common.username}</Text><span className="skeleton-line skeleton-username"/></div></div><Separator size="4"/><div className="credential-field"><div className="grow"><Text as="div" size="1" color="gray" className="credential-label">{copy.common.password}</Text><span className="skeleton-line skeleton-password"/></div></div></div>
    <Text as="p" size="1" color="gray" className="detail-loading-caption" role="status">{copy.accounts.loading}</Text>
  </div>;
}
