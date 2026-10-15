import { IconButton, Text, Tooltip } from '@radix-ui/themes';
import { ArrowDownToLine, ChevronRight } from 'lucide-react';
import type { Entry } from '../lib/types';
import { ActionIcon, EntryAvatar } from './Primitives';

export function AccountList({ title, entries, onOpen, onFill, busy }: { title: string; entries: Entry[]; onOpen(entry: Entry): void; onFill(entry: Entry): void; busy: boolean }) {
  return <section className="entry-group"><div className="group-heading"><Text size="1" weight="medium">{title}</Text><Text size="1">{entries.length}</Text></div>{entries.map(entry => <div key={entry.path} className="entry-row"><button className="entry-open" onClick={() => onOpen(entry)} disabled={busy}><EntryAvatar entry={entry}/><span className="entry-text"><Text as="span" size="2" weight="medium">{entry.title}</Text>{entry.folder && <Text as="span" size="1" color="gray">{entry.folder}</Text>}</span></button>{entry.matched ? <Tooltip content="填充到当前网站"><IconButton aria-label={`填充 ${entry.title}`} variant="soft" size="2" disabled={busy} onClick={() => onFill(entry)}><ArrowDownToLine size={16}/></IconButton></Tooltip> : <ActionIcon label={`查看 ${entry.title}`} onClick={() => onOpen(entry)} disabled={busy}><ChevronRight size={16}/></ActionIcon>}</div>)}</section>;
}
