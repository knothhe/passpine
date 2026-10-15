import { Flex, IconButton, Text, Tooltip } from '@radix-ui/themes';
import { Copy, TreePine } from 'lucide-react';
import type { Entry } from '../lib/types';

export function Brand() {
  return <Flex align="center" gap="2"><span className="brand-mark"><TreePine size={20}/></span><span className="brand-name">PassPine</span></Flex>;
}

export function ActionIcon({ label, children, onClick, disabled }: { label: string; children: React.ReactNode; onClick(): void; disabled?: boolean }) {
  return <Tooltip content={label}><IconButton aria-label={label} variant="ghost" color="gray" size="2" disabled={disabled} onClick={onClick}>{children}</IconButton></Tooltip>;
}

export function EntryAvatar({ entry, large }: { entry: Entry; large?: boolean }) {
  const name = entry.folder.split('/')[0] || entry.title;
  const color = [...name].reduce((sum, char) => sum + char.charCodeAt(0), 0) % 5;
  return <span className={`entry-avatar avatar-${color} ${large ? 'avatar-large' : ''}`}>{name.charAt(0).toUpperCase()}</span>;
}

export function SecretField({ label, value, onCopy }: { label: string; value: string; onCopy?: () => void }) {
  return <div className="credential-field"><div className="truncate"><Text as="div" size="1" color="gray" mb="1">{label}</Text><Text size="2">{value}</Text></div>{onCopy && <ActionIcon label={`复制${label}`} onClick={onCopy}><Copy size={16}/></ActionIcon>}</div>;
}
