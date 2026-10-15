import { useEffect, useState } from 'react';
import { Badge, Box, Button, Callout, Card, Flex, Heading, Separator, Text } from '@radix-ui/themes';
import { Check, ClipboardCopy, Command, Globe2, RefreshCw } from 'lucide-react';
import { Brand } from './Primitives';
import { errorMessage } from '../lib/errors';
import { HOST, type Client, type Snapshot } from '../lib/types';

export function Options({ client, extensionId, firefox = false }: { client: Client; extensionId: string; firefox?: boolean }) {
  const [snapshot, setSnapshot] = useState<Snapshot>(), [error, setError] = useState(''), [copied, setCopied] = useState(false);
  const refresh = async () => { try { setSnapshot(await client.request<Snapshot>({ action: 'snapshot' })); setError(''); } catch (error) { setSnapshot(undefined); setError(errorMessage(error)); } };
  useEffect(() => { void refresh(); }, []);
  return <main className="options-page">
    <header className="options-header"><Brand/><Heading size="3">浏览器设置</Heading>{client.demo && <Badge color="gray">示例数据</Badge>}</header>
    <Card size="3">
      <Flex gap="3" align="center"><Box className="grow"><Heading size="3">{snapshot ? snapshot.locked ? '已连接 · 已锁定' : '已连接' : '未连接客户端'}</Heading>{snapshot?.vaultName && <Text size="2" color="gray">{snapshot.vaultName}</Text>}</Box><Button variant="soft" onClick={() => void refresh()}><RefreshCw size={15}/>检查连接</Button></Flex>
      {error && <Callout.Root mt="4" size="1" color="amber"><Callout.Text>{error}</Callout.Text></Callout.Root>}
      {!snapshot && <><Separator size="4" my="4"/><div className="setup-steps"><SetupStep number="1">打开客户端，创建或导入密码库。</SetupStep><SetupStep number="2">在客户端设置中安装「浏览器连接」。</SetupStep><SetupStep number="3">解锁密码库，点击浏览器工具栏的 PassPine。</SetupStep></div></>}
    </Card>
    <Card size="3" mt="4">
      <Flex gap="2" align="center"><Command size={17}/><Heading size="3">快捷键</Heading></Flex>
      <div className="shortcut-row"><Text size="2">打开 PassPine</Text><kbd>⌘ / Ctrl ⇧ L</kbd></div>
      <div className="shortcut-row"><Text size="2">填充匹配账号</Text><kbd>⌘ / Ctrl ⇧ F</kbd></div>
      <Text as="p" size="1" color="gray" mt="3">{firefox ? '修改：about:addons → 管理扩展快捷键' : '修改：chrome://extensions/shortcuts'}</Text>
    </Card>
    <Card size="3" mt="4">
      <Flex gap="2" align="center"><Globe2 size={17}/><Heading size="3">连接信息</Heading></Flex>
      <div className="diagnostic-row"><Text color="gray" size="2">Native Host</Text><code>{HOST}</code></div>
      <div className="diagnostic-row"><Text color="gray" size="2">扩展 ID</Text><code>{extensionId}</code></div>
      <Flex justify="end" mt="3"><Button variant="ghost" size="1" onClick={() => { void client.copy(extensionId).then(() => setCopied(true)); }}>{copied ? <Check size={14}/> : <ClipboardCopy size={14}/>}复制扩展 ID</Button></Flex>
    </Card>
  </main>;
}

function SetupStep({ number, children }: { number: string; children: React.ReactNode }) {
  return <div className="setup-step"><span>{number}</span><Text size="2">{children}</Text></div>;
}
