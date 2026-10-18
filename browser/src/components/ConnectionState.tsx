import { useCopy } from './LanguageProvider';
import { Button, Heading, Text } from '@radix-ui/themes';
import { ArrowRight, RefreshCw, Unplug } from 'lucide-react';

export function ConnectionState({ error, retry, settings }: { error: string; retry(): void; settings(): void }) {
  const copy = useCopy();
  return <div className="empty-state"><div className="empty-icon"><Unplug size={30}/></div><Heading size="4">{copy.connection.disconnectedTitle}</Heading><Text size="2" color="gray">{error || copy.connection.openClient}</Text><Button onClick={retry}><RefreshCw size={15}/>{copy.connection.reconnect}</Button><Button variant="ghost" color="gray" onClick={settings}>{copy.connection.guide} <ArrowRight size={14}/></Button></div>;
}
