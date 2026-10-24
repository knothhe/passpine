import { useEffect, useState } from "react";
import {
  zh,
  en,
  resolveLanguage,
  type LanguagePreference,
} from "../constants/copy";
import type { PageId } from "../constants/routes";

const storageKey = "passpine-website-language";

export interface LanguageControl {
  preference: LanguagePreference;
  setPreference: (preference: LanguagePreference) => void;
}

// Prerendered pages have no visitor preference yet.
export const staticLanguage: LanguageControl = {
  preference: "system",
  setPreference: () => {},
};

export function useLanguage(page: PageId) {
  const [preference, setPreference] = useState<LanguagePreference>(() => {
    try {
      const saved = localStorage.getItem(storageKey);
      return saved === "en" || saved === "zh-CN" ? saved : "system";
    } catch {
      return "system";
    }
  });
  const [languages, setLanguages] = useState(() => navigator.languages);
  useEffect(() => {
    const update = () => setLanguages(navigator.languages);
    window.addEventListener("languagechange", update);
    return () => window.removeEventListener("languagechange", update);
  }, []);
  const language = resolveLanguage(preference, languages);
  const copy = language === "zh-CN" ? zh : en;
  const meta = copy.meta[page];
  useEffect(() => {
    document.documentElement.lang = language;
    document.title = meta.title;
    for (const selector of [
      'meta[name="description"]',
      'meta[property="og:description"]',
    ]) {
      document.querySelector(selector)?.setAttribute("content", meta.description);
    }
    document
      .querySelector('meta[property="og:title"]')
      ?.setAttribute("content", meta.title);
    try {
      localStorage.setItem(storageKey, preference);
    } catch {
      /* Storage can be disabled. */
    }
  }, [language, meta, preference]);
  return { copy, language, control: { preference, setPreference } };
}
