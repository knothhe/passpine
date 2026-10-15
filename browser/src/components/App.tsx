import { useCallback, useEffect, useRef, useState } from 'react';
import { AlertDialog, Badge, Box, Button, DropdownMenu, Flex, Heading, IconButton, SegmentedControl, Separator, Spinner, Text, TextField } from '@radix-ui/themes';
import { ArrowDownToLine, ArrowLeft, Check, Copy, Eye, EyeOff, Globe2, KeyRound, Link2, LockKeyhole, MoreHorizontal, Plus, RefreshCw, Search, Settings2, ShieldCheck, Shuffle, Trash2, TreePine, Unplug, X } from 'lucide-react';
import { entriesFor, parseDocument } from '../lib/model';
import { errorMessage } from '../lib/errors';
import type { Client, Entry, EntryDocument, SessionStatus, Snapshot } from '../lib/types';
import { ActionIcon, Brand, EntryAvatar, SecretField } from './Primitives';
import { ConnectionState } from './ConnectionState';
import { AccountList } from './AccountList';
import { EditorForm } from './EditorForm';
import { Generator } from './Generator';
import { SessionGuard } from '../lib/session';

export type Detail = EntryDocument & { entry: Entry; vaultId: string; receivedAt: number };
export type EditorValue = { vaultId: string; original?: Detail; password?: string };

export function App({ client }: { client: Client }) {
  const [snapshot, setSnapshot] = useState<Snapshot>();
  const [connectionError, setConnectionError] = useState('');
  const [query, setQuery] = useState('');
  const [scope, setScope] = useState<'all' | 'site'>('all');
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [notice, setNotice] = useState<{ text: string; error?: boolean }>();
  const [detail, setDetail] = useState<Detail>();
  const [revealed, setRevealed] = useState(false);
  const [editor, setEditor] = useState<EditorValue>();
  const [generatorOpen, setGeneratorOpen] = useState(false);
  const [deleteOpen, setDeleteOpen] = useState(false);

  const session = useRef(new SessionGuard());
  const latestSnapshot = useRef<Snapshot | undefined>(undefined);
  const detailRequest = useRef(0);
  const refreshPending = useRef<Promise<void> | undefined>(undefined);
  const [otpRemaining, setOtpRemaining] = useState(0);
  const clearSecrets = useCallback(() => {
    detailRequest.current++;
    setDetail(undefined); setRevealed(false); setEditor(undefined); setDeleteOpen(false); setGeneratorOpen(false);
  }, []);
  const disconnected = useCallback((error: unknown) => {
    session.current.disconnect(); latestSnapshot.current = undefined;
    setConnectionError(errorMessage(error)); setSnapshot(undefined); clearSecrets();
  }, [clearSecrets]);
  const observe = useCallback((state: SessionStatus, sequence: number) => {
    const result = session.current.observe(state, sequence);
    if (result.invalidated) clearSecrets();
    else if (result.metadataChanged) { detailRequest.current++; setDetail(undefined); setRevealed(false); }
    return result;
  }, [clearSecrets]);
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
      if (pending || refreshPending.current || !latestSnapshot.current || window.document.hidden) return;
      pending = true;
      void checkStatus().catch(disconnected).finally(() => { pending = false; });
    }, 2000);
    const visibility = () => { if (window.document.hidden) clearSecrets(); else if (latestSnapshot.current) void checkStatus().catch(disconnected); };
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
      }).catch(error => { if (!cancelled) { setDetail(undefined); setNotice({ text: errorMessage(error), error: true }); } });
    };
    tick(); const timer = setInterval(tick, 250);
    return () => { cancelled = true; clearInterval(timer); };
  }, [client, detail, editor, checkStatus]);
  useEffect(() => {
    const focusSearch = (event: KeyboardEvent) => { if ((event.metaKey || event.ctrlKey) && event.key === 'k') { event.preventDefault(); detailRequest.current++; setDetail(undefined); setTimeout(() => window.document.querySelector<HTMLInputElement>('input[aria-label="搜索密码库"]')?.focus(), 0); } };
    window.addEventListener('keydown', focusSearch); return () => window.removeEventListener('keydown', focusSearch);
  }, []);
  const run = async (operation: () => Promise<void>) => {
    if (busy) return;
    setBusy(true); setNotice(undefined);
    try { await operation(); } catch (error) { setNotice({ text: errorMessage(error), error: true }); }
    finally { setBusy(false); }
  };
  const open = (entry: Entry) => void run(async () => {
    if (!snapshot) return;
    const token = session.current.capture(), request = ++detailRequest.current;
    const doc = await client.request<EntryDocument>({ action: 'fetch', vaultId: snapshot.vaultId, entry: entry.path });
    await checkStatus();
    if (!session.current.valid(token) || request !== detailRequest.current) return;
    setDetail({ ...doc, entry, vaultId: snapshot.vaultId, receivedAt: Date.now() });
  });
  const fill = (entry: Entry) => void run(async () => {
    if (!snapshot?.tab) return;
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
  const entries = snapshot ? entriesFor(snapshot, query, scope) : [];
  const currentEntries = entries.filter(entry => entry.matched), otherEntries = entries.filter(entry => !entry.matched);
  const document = detail ? parseDocument(detail.contents) : undefined;

  return <div className="popup-shell">
    <header className="app-header"><Brand/><Flex gap="3" align="center">
      {snapshot && <span className={`connection-dot ${snapshot.locked ? 'locked' : ''}`} title={snapshot.locked ? '已锁定' : '客户端已连接'}/>}
      <ActionIcon label="设置与连接" onClick={() => void client.openOptions()}><Settings2 size={18}/></ActionIcon>
      {snapshot && !snapshot.locked && <ActionIcon label="锁定密码库" onClick={lock} disabled={busy}><LockKeyhole size={17}/></ActionIcon>}
    </Flex></header>

    {detail && document ? <>
      <div className="detail-heading"><ActionIcon label="返回密码库" onClick={() => { detailRequest.current++; setDetail(undefined); }}><ArrowLeft size={19}/></ActionIcon><DropdownMenu.Root><DropdownMenu.Trigger><IconButton aria-label="更多操作" variant="ghost" color="gray"><MoreHorizontal size={19}/></IconButton></DropdownMenu.Trigger><DropdownMenu.Content align="end"><DropdownMenu.Item onSelect={() => setEditor({ vaultId: detail.vaultId, original: detail })}>编辑账号</DropdownMenu.Item><DropdownMenu.Separator/><DropdownMenu.Item color="red" onSelect={() => setDeleteOpen(true)}>移入回收站</DropdownMenu.Item></DropdownMenu.Content></DropdownMenu.Root></div>
      <main className="detail-content">
        <div className="entry-summary"><EntryAvatar entry={detail.entry} large/><div><Heading size="4">{detail.entry.title}</Heading>{detail.entry.folder && <Text size="2" color="gray">{detail.entry.folder}</Text>}</div></div>
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
      </main>
      <footer className="detail-footer"><Button variant="soft" color="gray" onClick={() => setEditor({ vaultId: detail.vaultId, original: detail })} disabled={busy}>编辑</Button><Button className="grow" disabled={busy || !snapshot?.tab || !detail.entry.matched} onClick={() => fill(detail.entry)}><ArrowDownToLine size={16}/>{snapshot?.tab ? '填充' : '请先打开网站'}</Button></footer>
    </> : <>
      <div className="search-section"><TextField.Root aria-label="搜索密码库" placeholder="搜索账号" value={query} onChange={event => setQuery(event.target.value)} size="3" autoFocus><TextField.Slot><Search size={18}/></TextField.Slot><TextField.Slot>{query ? <IconButton aria-label="清除搜索" size="1" variant="ghost" onClick={() => setQuery('')}><X size={14}/></IconButton> : <kbd>⌘ K</kbd>}</TextField.Slot></TextField.Root></div>
      {loading ? <div className="empty-state"><Spinner size="3"/><Text size="2" color="gray">正在连接…</Text></div> : !snapshot ? <ConnectionState error={connectionError} retry={() => void refresh()} settings={() => void client.openOptions()}/> : !snapshot.configured ? <div className="empty-state"><div className="empty-icon"><TreePine size={30}/></div><Heading size="4">未设置密码库</Heading><Text size="2" color="gray">请在客户端创建或导入密码库。</Text><Button onClick={() => void refresh()}>重新连接</Button></div> : snapshot.locked ? <div className="empty-state"><div className="empty-icon"><LockKeyhole size={30}/></div><Heading size="4">密码库已锁定</Heading><Text size="2" color="gray">{snapshot.vaultName}</Text><Button disabled={busy} onClick={() => void run(async () => { await client.request({ action: 'unlock' }); await refresh(); })}><KeyRound size={16}/>前往客户端解锁</Button><Button variant="ghost" color="gray" onClick={() => void refresh()}>刷新</Button></div> : <>
        <div className="site-context"><Globe2 size={15}/><Text size="2">{snapshot.tab?.hostname || '未打开网站'}</Text>{snapshot.tab && !snapshot.tab.secure && <Badge variant="soft" color="amber" size="1">HTTP</Badge>}</div>
        <div className="list-toolbar"><SegmentedControl.Root size="1" value={scope} onValueChange={value => setScope(value as 'all' | 'site')}><SegmentedControl.Item value="all">全部账号 <span className="count">{snapshot.entries.length}</span></SegmentedControl.Item><SegmentedControl.Item value="site">当前网站 <span className="count">{snapshot.matches.length}</span></SegmentedControl.Item></SegmentedControl.Root><ActionIcon label="刷新密码库" onClick={() => void refresh()}><RefreshCw size={14}/></ActionIcon></div>
        <main className="entries-list">
          {currentEntries.length > 0 && <AccountList title="当前网站" entries={currentEntries} onOpen={open} onFill={fill} busy={busy}/>}
          {otherEntries.length > 0 && <AccountList title={query ? '搜索结果' : '其他账号'} entries={otherEntries} onOpen={open} onFill={fill} busy={busy}/>}
          {entries.length === 0 && <div className="no-results"><Search size={24}/><Text size="2">{query ? '没有找到匹配的账号' : scope === 'site' ? '此网站还没有关联账号' : '密码库中还没有账号'}</Text>{scope === 'site' && <Text size="1" color="gray">从全部账号中选择并关联。</Text>}</div>}
        </main>
        <div className="vault-status"><ShieldCheck size={13}/><Text size="1">{snapshot.vaultName}</Text></div>
      </>}
      <footer className="app-footer"><Button variant="soft" color="gray" onClick={() => setGeneratorOpen(true)}><Shuffle size={16}/>密码生成器</Button><Button disabled={!snapshot?.configured || snapshot.locked} onClick={() => { if (snapshot) setEditor({ vaultId: snapshot.vaultId }); }}><Plus size={17}/>新建账号</Button></footer>
    </>}
    {notice && <div className={`notice ${notice.error ? 'notice-error' : ''}`} role={notice.error ? 'alert' : 'status'}>{notice.error ? <Unplug size={16}/> : <Check size={16}/>}<span>{notice.text}</span><button aria-label="关闭提示" onClick={() => setNotice(undefined)}><X size={14}/></button></div>}
    {client.demo && <div className="demo-ribbon">预览 · 示例数据</div>}

    <EditorForm client={client} value={editor} defaultURL={snapshot?.tab ? `https://${snapshot.tab.hostname}` : ''} close={() => setEditor(undefined)} saved={async () => { setEditor(undefined); setDetail(undefined); await refresh(); setNotice({ text: '账号已加密保存' }); }}/>
    <Generator client={client} open={generatorOpen} close={() => setGeneratorOpen(false)} usePassword={password => { setGeneratorOpen(false); if (snapshot) setEditor({ vaultId: snapshot.vaultId, password }); }} canSave={!!snapshot?.configured && !snapshot.locked}/>
    <AlertDialog.Root open={deleteOpen} onOpenChange={setDeleteOpen}><AlertDialog.Content maxWidth="360px"><AlertDialog.Title>移入加密回收站？</AlertDialog.Title><AlertDialog.Description size="2">「{detail?.entry.title}」将从账号列表移除。你可以在客户端的回收站中恢复。</AlertDialog.Description><Flex justify="end" gap="3" mt="5"><AlertDialog.Cancel><Button variant="soft" color="gray">取消</Button></AlertDialog.Cancel><Button color="red" disabled={busy} onClick={() => void run(async () => { if (!detail) return; await client.request({ action: 'delete', vaultId: detail.vaultId, entry: detail.entry.path, revision: detail.revision }); setDeleteOpen(false); setDetail(undefined); await refresh(); setNotice({ text: '已移入加密回收站' }); })}><Trash2 size={15}/>移入回收站</Button></Flex></AlertDialog.Content></AlertDialog.Root>
  </div>;
}
