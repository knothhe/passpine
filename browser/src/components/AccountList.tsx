import { DropdownMenu, IconButton, Text, Tooltip } from '@radix-ui/themes';
import { ChevronRight, KeyRound, MoreHorizontal, UserRound } from 'lucide-react';
import type { Entry } from '../lib/types';

function EntryAction({ label, icon, disabled, onClick }: { label: string; icon: 'user' | 'password' | 'details'; disabled: boolean; onClick(): void }) {
  const Icon = { user: UserRound, password: KeyRound, details: ChevronRight }[icon];
  return <Tooltip content={label}><IconButton className="account-action" aria-label={label} variant="ghost" size="1" disabled={disabled} onClick={onClick}><Icon size={17}/></IconButton></Tooltip>;
}

export function AccountList({ entries, onOpen, onFill, onCopy, onAssociate, website, busy }: { entries: Entry[]; onOpen(entry: Entry): void; onFill(entry: Entry): void; onCopy(entry: Entry, field: 'username' | 'password'): void; onAssociate(entry: Entry): void; website?: string; busy: boolean }) {
  return <section className="account-list" aria-label="账号">{entries.map(entry => <div key={entry.path} className="entry-row"><button className="entry-open" data-entry-path={entry.path} title={entry.path} onClick={() => onFill(entry)} disabled={busy}><span className="entry-text"><span className="entry-path"><Text as="span" size="1" color="gray">{entry.folder || '密码库'}</Text>{entry.matched && website && <span className="site-account-badge" title={`可用于 ${website}`}>当前网站</span>}</span><Text as="span" size="2" weight="medium">{entry.title}</Text></span></button><div className="account-actions">
    <EntryAction label={`复制用户名 ${entry.title}`} icon="user" onClick={() => onCopy(entry, 'username')} disabled={busy}/>
    <EntryAction label={`复制密码 ${entry.title}`} icon="password" onClick={() => onCopy(entry, 'password')} disabled={busy}/>
    <EntryAction label={`查看 ${entry.title}`} icon="details" onClick={() => onOpen(entry)} disabled={busy}/>
    <DropdownMenu.Root><DropdownMenu.Trigger><IconButton className="account-action account-more" aria-label={`更多操作 ${entry.title}`} variant="ghost" size="1" disabled={busy}><MoreHorizontal size={18}/></IconButton></DropdownMenu.Trigger><DropdownMenu.Content align="end" size="1"><DropdownMenu.Label>{website || '当前没有网站'}</DropdownMenu.Label><DropdownMenu.Item disabled={!website || busy} onSelect={() => onAssociate(entry)}>{entry.matched ? '取消关联当前网站' : '关联到当前网站'}</DropdownMenu.Item></DropdownMenu.Content></DropdownMenu.Root>
  </div></div>)}</section>;
}
