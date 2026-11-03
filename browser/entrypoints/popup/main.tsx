import { lazy, Suspense } from 'react';
import { createRoot } from 'react-dom/client';
import { App } from '../../src/components/App';
import { Provider } from '../../src/components/Provider';
import { client } from '../../src/lib/client';

// This dynamic import is removed entirely from production extension builds.
const DevelopmentAudit = import.meta.env.DEV ? lazy(() => import('../../src/components/DevelopmentAudit')) : undefined;

createRoot(document.getElementById('root')!).render(<Provider><App client={client}/><Suspense fallback={null}>{DevelopmentAudit && <DevelopmentAudit surface="popup"/>}</Suspense></Provider>);
