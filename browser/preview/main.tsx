import { useState } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { Badge, Button, Flex, Text, Theme } from '@radix-ui/themes';
import { App } from '../src/components/App';
import { Options } from '../src/components/Options';
import { Provider } from '../src/components/Provider';
import { demoClient, setPreviewState } from './client';

const previewParameters = new URLSearchParams(location.search);
const requestedState = previewParameters.get('state');
const initialState = requestedState === 'locked' || requestedState === 'disconnected' ? requestedState : 'connected';
setPreviewState(initialState);

function Preview() {
  const [state, setState] = useState<'connected' | 'locked' | 'disconnected'>(initialState);
  if (previewParameters.get('view') === 'options') return <Options client={demoClient} extensionId="obnlebeedcbbchadakacilopfdaifgjh"/>;
  return <main className="preview-page">
    <header className="preview-toolbar"><Text size="2" weight="medium">扩展预览</Text><Badge color="gray">示例数据</Badge><a href="/?view=options">设置</a></header>
    <Flex gap="2" justify="center" className="preview-controls">
      {(['connected', 'locked', 'disconnected'] as const).map(value => <Button key={value} size="1" variant={state === value ? 'solid' : 'soft'} color={state === value ? undefined : 'gray'} onClick={() => { setPreviewState(value); setState(value); }}>{({ connected: '已连接', locked: '已锁定', disconnected: '未连接' })[value]}</Button>)}
    </Flex>
    <section className="preview-popup"><App key={state} client={demoClient}/></section>
  </main>;
}
const appearance = previewParameters.get('theme');
const root: Root = import.meta.hot?.data.reactRoot ?? createRoot(document.getElementById('root')!);
if (import.meta.hot) import.meta.hot.data.reactRoot = root;
root.render(<Provider>{appearance === 'light' || appearance === 'dark' ? <Theme appearance={appearance} accentColor="orange" grayColor="sand" radius="medium" scaling="100%" className="onevra-theme"><Preview/></Theme> : <Preview/>}</Provider>);
