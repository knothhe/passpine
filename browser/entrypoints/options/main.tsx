import { lazy, Suspense } from 'react';
import { createRoot } from 'react-dom/client';
import { browser } from 'wxt/browser';
import { Options } from '../../src/components/Options';
import { Provider } from '../../src/components/Provider';
import { client } from '../../src/lib/client';

// This dynamic import is removed entirely from production extension builds.
const DevelopmentAudit = import.meta.env.DEV ? lazy(() => import('../../src/components/DevelopmentAudit')) : undefined;

createRoot(document.getElementById('root')!).render(<Provider><Options client={client} extensionId={browser.runtime.id} firefox={browser.runtime.getURL('/').startsWith('moz-extension:')}/><Suspense fallback={null}>{DevelopmentAudit && <DevelopmentAudit surface="options"/>}</Suspense></Provider>);
