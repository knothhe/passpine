import { useEffect, useState } from 'react';
import { Theme } from '@radix-ui/themes';
import '@radix-ui/themes/styles.css';
import '../style.css';

export function Provider({ children }: { children: React.ReactNode }) {
  const [dark, setDark] = useState(() => matchMedia('(prefers-color-scheme: dark)').matches);
  useEffect(() => { const query = matchMedia('(prefers-color-scheme: dark)'); const changed = () => setDark(query.matches); query.addEventListener('change', changed); return () => query.removeEventListener('change', changed); }, []);
  return <Theme appearance={dark ? 'dark' : 'light'} accentColor="orange" grayColor="sand" radius="medium" scaling="100%" className="onevra-theme">{children}</Theme>;
}
