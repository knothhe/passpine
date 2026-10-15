import { Flex, Heading, IconButton, Text, Tooltip } from '@radix-ui/themes';
import { ArrowLeft, Copy } from 'lucide-react';
import type { Entry } from '../lib/types';
import appIconURL from '../assets/app-icon.png';

export function ApplicationIcon({ size = 24 }: { size?: number }) {
  return <img src={appIconURL} alt="" width={size} height={size} aria-hidden="true" style={{ display: 'block', objectFit: 'contain', flexShrink: 0 }}/>;
}

export function Brand() {
  return <Flex align="center" gap="2"><span className="brand-mark" style={{ background: 'transparent', width: 30, height: 30 }}><ApplicationIcon size={30}/></span><span className="brand-name">PassPine</span></Flex>;
}

export function ActionIcon({ label, children, onClick, disabled }: { label: string; children: React.ReactNode; onClick(): void; disabled?: boolean }) {
  return <Tooltip content={label}><IconButton aria-label={label} variant="ghost" color="gray" size="2" disabled={disabled} onClick={onClick}>{children}</IconButton></Tooltip>;
}

export function PageHeader({ title, backLabel = '返回密码库', back, disabled, children }: { title: string; backLabel?: string; back(): void; disabled?: boolean; children?: React.ReactNode }) {
  return <header className="screen-header"><div className="screen-header-title"><ActionIcon label={backLabel} onClick={back} disabled={disabled}><ArrowLeft size={18}/></ActionIcon><Heading as="h1" size="3" tabIndex={-1}>{title}</Heading></div>{children && <div className="screen-header-actions">{children}</div>}</header>;
}

export function EntryAvatar({ entry, large }: { entry: Entry; large?: boolean }) {
  const name = entry.folder.split('/')[0] || entry.title;
  const color = [...name].reduce((sum, char) => sum + char.charCodeAt(0), 0) % 5;
  return <span className={`entry-avatar avatar-${color} ${large ? 'avatar-large' : ''}`}>{name.charAt(0).toUpperCase()}</span>;
}

export function SecretField({ label, value, onCopy }: { label: string; value: string; onCopy?: () => void }) {
  return <div className="credential-field"><div className="truncate"><Text as="div" size="1" color="gray" mb="1">{label}</Text><Text size="2">{value}</Text></div>{onCopy && <ActionIcon label={`复制${label}`} onClick={onCopy}><Copy size={16}/></ActionIcon>}</div>;
}
