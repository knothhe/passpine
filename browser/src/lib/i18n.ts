import { chineseCopy, type Copy } from '../constants/copy';
import { englishCopy } from '../i18n/en';

export type Locale = 'en' | 'zh-CN';
export type LanguagePreference = 'system' | Locale;
export const systemLocale = () => typeof navigator === 'undefined' ? '' : navigator.language;

// Match client/i18n.Resolve, including explicit scripts and unsupported locales.
export function resolveLocale(preference: string = 'system', system = systemLocale()): Locale {
  const locale = (!preference || preference === 'system' ? system : preference).trim().toLowerCase().replaceAll('_', '-').split(/[.@]/)[0] ?? '';
  const [language, ...parts] = locale.split('-');
  if (language !== 'zh') return 'en';
  for (const part of parts) {
    if (part === 'hans') return 'zh-CN';
    if (part.length === 4) return 'en';
  }
  return parts.some(part => ['tw', 'hk', 'mo'].includes(part)) ? 'en' : 'zh-CN';
}

export const copyFor = (locale: Locale): Copy => locale === 'zh-CN' ? chineseCopy : englishCopy;
export const systemCopy = (): Copy => copyFor(resolveLocale());
