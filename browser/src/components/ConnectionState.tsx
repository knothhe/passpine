import { Button, Heading, Text } from '@radix-ui/themes';
import { ArrowRight, RefreshCw, Unplug } from 'lucide-react';

export function ConnectionState({ error, retry, settings }: { error: string; retry(): void; settings(): void }) {
  return <div className="empty-state"><div className="empty-icon"><Unplug size={30}/></div><Heading size="4">未连接客户端</Heading><Text size="2" color="gray">{error || '请打开 Passpine 客户端。'}</Text><Button onClick={retry}><RefreshCw size={15}/>重新连接</Button><Button variant="ghost" color="gray" onClick={settings}>连接指引 <ArrowRight size={14}/></Button></div>;
}
