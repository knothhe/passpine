import { createRoot } from 'react-dom/client';
import { App } from '../../src/components/App';
import { Provider } from '../../src/components/Provider';
import { client } from '../../src/lib/client';

createRoot(document.getElementById('root')!).render(<Provider><App client={client}/></Provider>);
