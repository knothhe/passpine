import { createContext, useContext, useEffect, useLayoutEffect, useRef, useState } from 'react';
import { Theme } from '@radix-ui/themes';
import { LanguageProvider } from './LanguageProvider';
import { defaultAppearancePreferences, normalizeAppearancePreferences, readAppearancePreferences, watchAppearancePreferences, writeAppearancePreferences, type AppearancePreferences } from '../lib/appearance';
import appIconURL from '../assets/app-icon.png';
import monochromeIconURL from '../assets/app-icon-monochrome.png';
import '@radix-ui/themes/styles.css';
import './theme.css';
import '../style.css';

const AppearanceContext = createContext({
  preferences: defaultAppearancePreferences,
  resolvedAppearance: 'light' as 'light' | 'dark',
  saving: false,
  setPreferences: async (_preferences: AppearancePreferences): Promise<void> => {},
});
export const useAppearance = () => useContext(AppearanceContext);

export function Provider({ children, demo = false, initialPreferences }: { children: React.ReactNode; demo?: boolean; initialPreferences?: Partial<AppearancePreferences> }) {
  const [preferences, setPreferences] = useState(() => normalizeAppearancePreferences({ ...defaultAppearancePreferences, ...initialPreferences }));
  const [saving, setSaving] = useState(false);
  const revision = useRef(0);
  const [dark, setDark] = useState(() => matchMedia('(prefers-color-scheme: dark)').matches);
  useEffect(() => { const query = matchMedia('(prefers-color-scheme: dark)'); const changed = () => setDark(query.matches); query.addEventListener('change', changed); return () => query.removeEventListener('change', changed); }, []);
  useEffect(() => {
    let active = true;
    const unwatch = watchAppearancePreferences(next => { revision.current++; if (active) setPreferences(next); }, demo);
    const initialRevision = revision.current;
    void readAppearancePreferences(demo).then(async next => {
      if (!active || revision.current !== initialRevision) return;
      const seeded = demo && initialPreferences ? normalizeAppearancePreferences({ ...next, ...initialPreferences }) : next;
      setPreferences(seeded);
      if (demo && initialPreferences) await writeAppearancePreferences(seeded, true);
    }).catch(() => {});
    return () => { active = false; revision.current++; unwatch(); };
  }, [demo, initialPreferences]);
  const resolvedAppearance = preferences.appearance === 'system' ? dark ? 'dark' : 'light' : preferences.appearance;
  useLayoutEffect(() => {
    document.documentElement.style.colorScheme = resolvedAppearance;
    document.documentElement.style.setProperty('--pine-font-scale', String(preferences.fontScale / 100));
    document.documentElement.dataset.fontScale = String(preferences.fontScale);
    document.documentElement.dataset.themeStyle = preferences.style;
    document.documentElement.dataset.appearance = resolvedAppearance;
    const favicon = document.querySelector<HTMLLinkElement>('link[rel="icon"]');
    if (favicon) favicon.href = preferences.style === 'monochrome' ? monochromeIconURL : appIconURL;
  }, [preferences.style, preferences.fontScale, resolvedAppearance]);
  const save = async (next: AppearancePreferences) => {
    const previous = preferences;
    const requestRevision = ++revision.current;
    setPreferences(normalizeAppearancePreferences(next));
    setSaving(true);
    try {
      await writeAppearancePreferences(next, demo);
      // Storage events already applied this write or a newer cross-context value.
      if (revision.current === requestRevision) setPreferences(normalizeAppearancePreferences(next));
    }
    catch (error) {
      if (revision.current === requestRevision) setPreferences(previous);
      throw error;
    }
    finally { setSaving(false); }
  };
  return <LanguageProvider demo={demo}><AppearanceContext.Provider value={{ preferences, resolvedAppearance, saving, setPreferences: save }}><Theme appearance={resolvedAppearance} accentColor={preferences.style === 'monochrome' ? 'gray' : 'orange'} grayColor={preferences.style === 'monochrome' ? 'gray' : 'sand'} radius="medium" scaling="100%">{children}</Theme></AppearanceContext.Provider></LanguageProvider>;
}
