import { useCallback, useEffect, useLayoutEffect, useRef, useState } from 'react';
import { AnimatePresence, motion, MotionConfig, useIsPresent } from 'motion/react';
import { AlertDialog, Badge, Box, Button, DropdownMenu, Flex, Heading, IconButton, Separator, Spinner, Text, TextField, Tooltip } from '@radix-ui/themes';
import { ArrowDownToLine, Check, Copy, Eye, EyeOff, Globe2, KeyRound, Link2, LockKeyhole, MoreHorizontal, Plus, RefreshCw, Search, Settings2, Trash2, Unplug, X } from 'lucide-react';
import { entriesFor, parseDocument } from '../lib/model';
import { errorMessage } from '../lib/errors';
import type { Client, Entry, EntryDocument, SessionStatus, Snapshot } from '../lib/types';
import { ActionIcon, ApplicationIcon, Brand, EntryAvatar, PageHeader, SecretField } from './Primitives';
import { ConnectionState } from './ConnectionState';
import { AccountList } from './AccountList';
import { EditorForm } from './EditorForm';
import { SessionGuard } from '../lib/session';

export type Detail = EntryDocument & { entry: Entry; vaultId: string; receivedAt: number };
export type EditorValue = { vaultId: string; original?: Detail; password?: string };
type DetailTarget = { entry: Entry; vaultId: string };

export function App({ client }: { client: Client }) {
  const [snapshot, setSnapshot] = useState<Snapshot>();
  const [connectionError, setConnectionError] = useState('');
  const [query, setQuery] = useState('');
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [notice, setNotice] = useState<{ text: string; error?: boolean }>();
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
  const refreshPending = useRef<Promise<void> | undefined>(undefined);
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
    setConnectionError(errorMessage(error)); setSnapshot(undefined); clearSecrets();
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
  const refresh = useCallback((): Promise<void> => {
    if (refreshPending.current) return refreshPending.current;
    const task = (async () => {
      const sequence = session.current.probe();
      try {
        const next = await client.request<Snapshot>({ action: 'snapshot' });
        if (!observe(next, sequence).accepted) return;
        latestSnapshot.current = next; setSnapshot(next); setConnectionError('');
      } catch (error) { disconnected(error); }
      finally { setLoading(false); }
    })();
    refreshPending.current = task;
    void task.finally(() => { if (refreshPending.current === task) refreshPending.current = undefined; });
    return task;
  }, [client, observe, disconnected]);
  const checkStatus = useCallback(async () => {
    const sequence = session.current.probe();
    const state = await client.request<SessionStatus>({ action: 'status' });
    const observation = observe(state, sequence);
    const previous = latestSnapshot.current;
    const needsSnapshot = !previous || previous.vaultId !== state.vaultId || previous.locked !== state.locked || previous.configured !== state.configured || previous.entriesVersion !== state.entriesVersion;
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
  useEffect(() => { if (!notice) return; const timer = setTimeout(() => setNotice(undefined), 4500); return () => clearTimeout(timer); }, [notice]);
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
      }).catch(error => { if (!cancelled) { cancelDetails(); setNotice({ text: errorMessage(error), error: true }); } });
    };
    tick(); const timer = setInterval(tick, 250);
    return () => { cancelled = true; clearInterval(timer); };
  }, [client, detail, editor, checkStatus, cancelDetails]);
  useEffect(() => {
    const focusSearch = (event: KeyboardEvent) => { if ((event.metaKey || event.ctrlKey) && event.key === 'k') { event.preventDefault(); if (editor) return; cancelDetails(); setTimeout(() => window.document.querySelector<HTMLInputElement>('input[aria-label="搜索密码库"]')?.focus(), 0); } };
    window.addEventListener('keydown', focusSearch); return () => window.removeEventListener('keydown', focusSearch);
  }, [editor, cancelDetails]);
  const run = async (operation: () => Promise<void>) => {
    if (busyOperation.current) return;
    const request = ++operationRequest.current;
    busyOperation.current = true;
    setBusy(true); setNotice(undefined);
    try { await operation(); } catch (error) { if (request === operationRequest.current) setNotice({ text: errorMessage(error), error: true }); }
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
    if (!state || state.vaultId !== vaultId) throw new Error('当前密码库已切换，请重新选择账号。');
    if (state.locked) throw new Error('密码库尚未解锁，请在客户端完成验证。');
  };
  const copyEntry = (entry: Entry, field: 'password' | 'username') => void run(async () => {
    if (!snapshot) return;
    await ensureUnlocked(snapshot.vaultId);
    await client.request({ action: 'copy', vaultId: snapshot.vaultId, entry: entry.path, field });
    setNotice({ text: client.demo ? '示例数据：未复制凭证' : '已复制' });
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
        if (!doc.session.configured || doc.session.locked || doc.session.vaultId !== vaultId) throw new Error('密码库会话已变化，请重新打开账号。');
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
    await ensureUnlocked(snapshot.vaultId);
    const result = await client.request<{ filled: number }>({ action: 'fill', vaultId: snapshot.vaultId, entry: entry.path, tab: snapshot.tab });
    setNotice({ text: client.demo ? `演示：将填写 ${result.filled} 个登录字段` : `已填写 ${result.filled} 个登录字段` });
  });
  const copy = (field: 'password' | 'username' | 'otp') => void run(async () => {
    if (!detail) return;
    await client.request({ action: 'copy', vaultId: detail.vaultId, entry: detail.entry.path, field });
    setNotice({ text: client.demo ? '示例数据：未复制凭证' : '已复制 · 按客户端设置自动清空' });
  });
  const lock = () => void run(async () => {
    clearSecrets(); session.current.disconnect();
    await client.request({ action: 'lock' }); await refresh();
  });
  const entries = snapshot ? entriesFor(snapshot, query) : [];
  const associate = (entry: Entry) => void run(async () => {
    if (!snapshot?.tab) return;
    await client.request({ action: 'associate', vaultId: snapshot.vaultId, entry: entry.path, tab: snapshot.tab });
    await refresh(); setNotice({ text: `已关联 ${snapshot.tab.hostname}` });
  });
  const document = detail ? parseDocument(detail.contents) : undefined;
  const detailEntry = detail?.entry ?? detailTarget?.entry;
  const page = editor ? 'editor' : detailEntry ? 'detail' : 'list';

  return <MotionConfig reducedMotion="user" transition={{ duration: .16, ease: [.2, .8, .2, 1] }}><div className="popup-shell">
    <header className="app-header"><Brand/><Flex gap="2" align="center" className="header-actions">
      {snapshot && <>{snapshot.vaultName && <span className="header-vault" title={snapshot.vaultName}>{snapshot.vaultName}</span>}<span className="connection-label"><span className={`connection-dot ${snapshot.locked ? 'locked' : ''}`}/>{snapshot.locked ? '已锁定' : '已连接'}</span></>}
      <ActionIcon label="设置与连接" onClick={() => void client.openOptions()}><Settings2 size={18}/></ActionIcon>
      {snapshot?.configured && !snapshot.locked && <ActionIcon label="锁定密码库" onClick={lock} disabled={busy}><LockKeyhole size={17}/></ActionIcon>}
    </Flex></header>

    <div className="popup-pages"><AnimatePresence initial={false} mode="sync"><PopupPage key={page} view={page} listScroll={listScroll.current} returnEntry={returnEntry.current}>
    {editor ? <EditorForm client={client} value={editor} defaultURL={snapshot?.tab ? `https://${snapshot.tab.hostname}` : ''} close={() => setEditor(undefined)} saved={async () => { setEditor(undefined); cancelDetails(); await refresh(); setNotice({ text: '账号已加密保存' }); }}/>
    : detailEntry ? <>
      <PageHeader title="账号详情" back={cancelDetails}><DropdownMenu.Root><DropdownMenu.Trigger><IconButton aria-label="更多操作" variant="ghost" color="gray" disabled={!detail || busy}><MoreHorizontal size={19}/></IconButton></DropdownMenu.Trigger><DropdownMenu.Content align="end"><DropdownMenu.Item onSelect={() => detail && setEditor({ vaultId: detail.vaultId, original: detail })}>编辑账号</DropdownMenu.Item><DropdownMenu.Separator/><DropdownMenu.Item color="red" onSelect={() => setDeleteOpen(true)}>移入回收站</DropdownMenu.Item></DropdownMenu.Content></DropdownMenu.Root></PageHeader>
      <main className="screen-content detail-content" aria-busy={!detail}>
        <div className="entry-summary"><EntryAvatar entry={detailEntry} large/><div><Heading size="4">{detailEntry.title}</Heading>{detailEntry.folder && <Text size="2" color="gray">{detailEntry.folder}</Text>}</div></div>
        <div className="detail-content-swap"><AnimatePresence initial={false}>{!detail && <motion.div className="detail-loading-layer" key="loading" exit={{ opacity: 0 }} transition={{ duration: .1 }}><DetailSkeleton/></motion.div>}</AnimatePresence>
        {detail && document && <motion.div className="detail-data-layer" initial={{ opacity: 0 }} animate={{ opacity: 1 }} transition={{ duration: .1 }}>
        <div className="credential-card">
          <SecretField label="用户名" value={document.username || '未设置'} onCopy={document.username ? () => copy('username') : undefined}/>
          <Separator size="4"/>
          <div className="credential-field"><div><Text as="div" size="1" color="gray" mb="1">密码</Text><Text className="secret-text" size="3">{revealed ? document.password : '••••••••••••••••'}</Text></div><Flex gap="3"><ActionIcon label={revealed ? '隐藏密码' : '显示密码'} onClick={() => setRevealed(!revealed)}>{revealed ? <EyeOff size={16}/> : <Eye size={16}/>}</ActionIcon><ActionIcon label="复制密码" onClick={() => copy('password')}><Copy size={16}/></ActionIcon></Flex></div>
          {detail.otp && <><Separator size="4"/><SecretField label={`一次性验证码 · ${otpRemaining > 0 ? `${otpRemaining} 秒` : '刷新中'}`} value={otpRemaining > 0 ? detail.otp : '••••••'} onCopy={() => copy('otp')}/></>}
        </div>
        <Text as="div" size="1" color="gray" weight="medium" mt="4" mb="2">网站</Text>
        <div className="site-detail"><Globe2 size={16}/><Text size="2" className="truncate">{document.urls[0] || (detail.entry.matched ? snapshot?.tab?.hostname : '尚未设置网站')}</Text>{detail.entry.matched && <Check size={15} className="accent"/>}</div>
        {snapshot?.tab && !detail.entry.matched && <Button variant="soft" size="2" mt="3" onClick={() => void run(async () => {
          await client.request({ action: 'associate', vaultId: detail.vaultId, entry: detail.entry.path, tab: snapshot.tab! });
          setDetail(previous => previous?.entry.path === detail.entry.path && previous.vaultId === detail.vaultId ? { ...previous, entry: { ...previous.entry, matched: true } } : previous); await refresh(); setNotice({ text: `已关联 ${snapshot.tab!.hostname}` });
        })}><Link2 size={14}/>关联到 {snapshot.tab.hostname}</Button>}
        {document.notes && <Box mt="4"><Text as="div" size="1" color="gray" weight="medium" mb="2">备注</Text><Text as="p" size="2" className="notes">{document.notes}</Text></Box>}
        </motion.div>}</div>
      </main>
      <footer className="screen-footer detail-footer"><Button variant="soft" color="gray" onClick={() => detail && setEditor({ vaultId: detail.vaultId, original: detail })} disabled={busy || !detail}>编辑</Button><Button className="grow" disabled={busy || !detail || !snapshot?.tab || !detail.entry.matched} onClick={() => detail && fill(detail.entry)}><ArrowDownToLine size={16}/>{snapshot?.tab ? '填充' : '请先打开网站'}</Button></footer>
    </> : <>
      <div className="site-context"><Text size="1" color="gray">当前网站</Text><Text size="2" weight="medium" className="truncate">{snapshot?.tab?.hostname || '未打开网站'}</Text>{snapshot?.tab && !snapshot.tab.secure && <Badge variant="soft" color="amber" size="1">HTTP</Badge>}<Tooltip content="新建账号"><IconButton className="site-create" aria-label="新建账号" size="1" onClick={() => void run(async () => { if (!snapshot?.configured) return; await ensureUnlocked(snapshot.vaultId); setEditor({ vaultId: snapshot.vaultId }); })} disabled={!snapshot?.configured || busy}><Plus size={16}/></IconButton></Tooltip></div>
      <div className="search-section"><TextField.Root aria-label="搜索密码库" placeholder="搜索账号" value={query} onChange={event => setQuery(event.target.value)} size="3" autoFocus><TextField.Slot><Search size={18}/></TextField.Slot><TextField.Slot>{query ? <IconButton aria-label="清除搜索" size="1" variant="ghost" onClick={() => setQuery('')}><X size={14}/></IconButton> : <kbd>⌘ K</kbd>}</TextField.Slot></TextField.Root></div>
      {loading ? <div className="empty-state"><Spinner size="3"/><Text size="2" color="gray">正在连接…</Text></div> : !snapshot ? <ConnectionState error={connectionError} retry={() => void refresh()} settings={() => void client.openOptions()}/> : !snapshot.configured ? <div className="empty-state"><div className="empty-icon"><ApplicationIcon size={40}/></div><Heading size="4">未设置密码库</Heading><Text size="2" color="gray">请在客户端创建或导入密码库。</Text><Button onClick={() => void refresh()}>重新连接</Button></div> : snapshot.locked && snapshot.entries.length === 0 ? <div className="empty-state"><div className="empty-icon"><LockKeyhole size={30}/></div><Heading size="4">密码库已锁定</Heading><Text size="2" color="gray">{snapshot.vaultName}</Text><Button disabled={busy} onClick={() => void run(unlock)}><KeyRound size={16}/>前往客户端解锁</Button><Button variant="ghost" color="gray" onClick={() => void refresh()}>刷新</Button></div> : <>
        <div className="list-toolbar"><Text size="1" color="gray">{query ? '搜索结果' : '全部账号'}</Text><Text size="1" color="gray">{entries.length} 个账号</Text><ActionIcon label="刷新密码库" onClick={() => void refresh()}><RefreshCw size={14}/></ActionIcon></div>
        <main className="entries-list">
          {entries.length > 0 && <AccountList entries={entries} onOpen={open} onFill={fill} onCopy={copyEntry} onAssociate={associate} website={snapshot.tab?.hostname} busy={busy}/>}
          {entries.length === 0 && <div className="no-results"><Search size={24}/><Text size="2">{query ? '没有找到匹配的账号' : '密码库中还没有账号'}</Text></div>}
        </main>
      </>}
    </>}
    </PopupPage></AnimatePresence></div>
    {notice && <div className={`notice ${notice.error ? 'notice-error' : ''}`} role={notice.error ? 'alert' : 'status'}>{notice.error ? <Unplug size={16}/> : <Check size={16}/>}<span>{notice.text}</span><button aria-label="关闭提示" onClick={() => setNotice(undefined)}><X size={14}/></button></div>}
    {client.demo && <div className="demo-ribbon">预览 · 示例数据</div>}

    <AlertDialog.Root open={deleteOpen} onOpenChange={setDeleteOpen}><AlertDialog.Content maxWidth="360px"><AlertDialog.Title>移入加密回收站？</AlertDialog.Title><AlertDialog.Description size="2">「{detail?.entry.title}」将从账号列表移除。你可以在客户端的回收站中恢复。</AlertDialog.Description><Flex justify="end" gap="3" mt="5"><AlertDialog.Cancel><Button variant="soft" color="gray">取消</Button></AlertDialog.Cancel><Button color="red" disabled={busy} onClick={() => void run(async () => { if (!detail) return; await client.request({ action: 'delete', vaultId: detail.vaultId, entry: detail.entry.path, revision: detail.revision }); setDeleteOpen(false); cancelDetails(); await refresh(); setNotice({ text: '已移入加密回收站' }); })}><Trash2 size={15}/>移入回收站</Button></Flex></AlertDialog.Content></AlertDialog.Root>
  </div></MotionConfig>;
}

function PopupPage({ view, children, listScroll, returnEntry }: { view: 'list' | 'detail' | 'editor'; children: React.ReactNode; listScroll: number; returnEntry?: string }) {
  const present = useIsPresent();
  const ref = useRef<HTMLElement>(null);
  useLayoutEffect(() => {
    if (!present || window.document.hidden) return;
    const page = ref.current;
    if (view === 'detail') page?.querySelector<HTMLHeadingElement>('h1')?.focus({ preventScroll: true });
    else if (view === 'list' && returnEntry) {
      const list = page?.querySelector<HTMLElement>('.entries-list');
      if (list) list.scrollTop = listScroll;
      const entry = Array.from(page?.querySelectorAll<HTMLButtonElement>('button[data-entry-path]') ?? []).find(button => button.dataset.entryPath === returnEntry);
      (entry ?? page?.querySelector<HTMLInputElement>('input[aria-label="搜索密码库"]'))?.focus({ preventScroll: true });
    }
  }, [present, view]);
  const offset = view === 'list' ? -12 : 12;
  return <motion.section ref={ref} className={`popup-page popup-page-${view}`} data-popup-page={view} inert={!present} aria-hidden={!present} initial={{ opacity: 0, x: offset }} animate={{ opacity: 1, x: 0 }} exit={{ opacity: 0, x: offset, transition: { duration: .1 } }}>
    {/* AnimatePresence retains removed children. Drop credentials immediately,
        leaving only an empty surface to animate after navigation or a lock. */}
    {present || view === 'list' ? children : <div className="page-transition-mask"/>}
  </motion.section>;
}

function DetailSkeleton() {
  return <div className="detail-skeleton">
    <div className="credential-card" aria-hidden="true"><div className="credential-field"><div className="grow"><Text as="div" size="1" color="gray" mb="2">用户名</Text><span className="skeleton-line skeleton-username"/></div></div><Separator size="4"/><div className="credential-field"><div className="grow"><Text as="div" size="1" color="gray" mb="2">密码</Text><span className="skeleton-line skeleton-password"/></div></div></div>
    <Text as="p" size="1" color="gray" className="detail-loading-caption" role="status">正在读取账号…</Text>
  </div>;
}
