import { createRoot } from 'react-dom/client';
import { browser } from 'wxt/browser';
import { Options } from '../../src/components/Options';
import { Provider } from '../../src/components/Provider';
import { client } from '../../src/lib/client';

createRoot(document.getElementById('root')!).render(<Provider><Options client={client} extensionId={browser.runtime.id} firefox={browser.runtime.getURL('/').startsWith('moz-extension:')}/></Provider>);
