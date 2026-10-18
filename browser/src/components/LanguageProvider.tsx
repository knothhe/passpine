import { createContext, useContext, useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react';
import type { Copy } from '../constants/copy';
import { copyFor, resolveLocale, systemLocale, type LanguagePreference, type Locale } from '../lib/i18n';
import { readLanguage, watchLanguage, writeLanguage } from '../lib/language';

interface LanguageState {
  language: LanguagePreference;
  locale: Locale;
  catalog: Copy;
  loading: boolean;
  saving: boolean;
  loadFailed: boolean;
  setLanguage(language: LanguagePreference): Promise<void>;
}
const LanguageContext = createContext<LanguageState | null>(null);
export function useLanguage(): LanguageState {
  const state = useContext(LanguageContext);
  const locale = resolveLocale();
  return state ?? { language: 'system', locale, catalog: copyFor(locale), loading: false, saving: false, loadFailed: false, setLanguage: async () => {} };
}

export function useCopy(): Copy {
  const { catalog } = useLanguage();
  const latest = useRef(catalog);
  latest.current = catalog;
  // Stable callback references always read current messages. A language change
  // must not restart vault requests, clear editor inputs or regenerate passwords.
  return useMemo(() => new Proxy(catalog, { get: (_target, key) => Reflect.get(latest.current, key) }), []);
}

export function LanguageProvider({ children, demo = false }: { children: React.ReactNode; demo?: boolean }) {
  const [language, setLanguage] = useState<LanguagePreference>('system');
  const [system, setSystem] = useState(systemLocale);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false), [loadFailed, setLoadFailed] = useState(false);
  const revision = useRef(0), active = useRef(false);
  useEffect(() => {
    active.current = true;
    setLoading(true);
    const unwatch = watchLanguage(next => { revision.current++; if (active.current) { setLanguage(next); setLoadFailed(false); setLoading(false); } }, demo);
    const initial = revision.current;
    void readLanguage(demo).then(next => {
      if (active.current && revision.current === initial) { setLanguage(next); setLoadFailed(false); setLoading(false); }
    }).catch(() => { if (active.current && revision.current === initial) { setLoadFailed(true); setLoading(false); } });
    const changed = () => setSystem(systemLocale());
    window.addEventListener('languagechange', changed);
    return () => { active.current = false; revision.current++; unwatch(); window.removeEventListener('languagechange', changed); };
  }, [demo]);
  const locale = resolveLocale(language, system), catalog = copyFor(locale);
  useLayoutEffect(() => {
    document.documentElement.lang = locale;
    const title = document.documentElement.dataset.copyTitle;
    if (title) document.title = title === 'options' ? catalog.options.pageTitle : title === 'preview' ? catalog.preview.pageTitle : catalog.brand;
  }, [locale, catalog]);
  const save = async (next: LanguagePreference) => {
    const previous = language, request = ++revision.current;
    setLanguage(next); setSaving(true);
    try { await writeLanguage(next, demo); if (active.current) setLoadFailed(false); }
    catch (error) { if (active.current && revision.current === request) setLanguage(previous); throw error; }
    finally { if (active.current) setSaving(false); }
  };
  return <LanguageContext.Provider value={{ language, locale, catalog, loading, saving, loadFailed, setLanguage: save }}>{children}</LanguageContext.Provider>;
}
