import { Flex, Heading, IconButton, Text, Tooltip } from '@radix-ui/themes';
import { ArrowLeft, Copy } from 'lucide-react';
import type { Entry } from '../lib/types';
import appIconURL from '../assets/app-icon.png';
import monochromeIconURL from '../assets/app-icon-monochrome.png';
import { useAppearance } from './Provider';
import type { ThemeStyle } from '../lib/appearance';

export function ApplicationIcon({ size = 24, style }: { size?: number; style?: ThemeStyle }) {
  const { preferences } = useAppearance();
  return <img src={(style ?? preferences.style) === 'monochrome' ? monochromeIconURL : appIconURL} alt="" width={size} height={size} aria-hidden="true" style={{ display: 'block', objectFit: 'contain', flexShrink: 0 }}/>;
}

export function Brand() {
  return <Flex align="center" gap="2"><span className="brand-mark" style={{ background: 'transparent', width: 30, height: 30 }}><ApplicationIcon size={30}/></span><span className="brand-name">Passpine</span></Flex>;
}

export function ActionIcon({ label, children, onClick, disabled, loading }: { label: string; children: React.ReactNode; onClick(): void; disabled?: boolean; loading?: boolean }) {
  return <Tooltip content={label}><IconButton aria-label={label} aria-busy={loading || undefined} variant="ghost" color="gray" size="2" disabled={disabled} onClick={onClick}>{children}</IconButton></Tooltip>;
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
  return <div className="credential-field"><div><Text as="div" size="1" color="gray" className="credential-label">{label}</Text><Text as="div" size="2" className="credential-value truncate" title={value}>{value}</Text></div>{onCopy && <ActionIcon label={`复制${label}`} onClick={onCopy}><Copy size={16}/></ActionIcon>}</div>;
}
