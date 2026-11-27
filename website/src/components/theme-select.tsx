import { useEffect, useState } from "react";
import { SunMoon } from "lucide-react";
import type { Copy } from "../constants/copy";

type Theme = "system" | "light" | "dark";
const storageKey = "passpine-website-theme";

function preference(value: string | null | undefined): Theme {
  return value === "light" || value === "dark" ? value : "system";
}

function applyTheme(theme: Theme) {
  if (theme === "system") delete document.documentElement.dataset.theme;
  else document.documentElement.dataset.theme = theme;
}

export function ThemeSelect({ c }: { c: Copy }) {
  const [theme, setTheme] = useState<Theme>(() =>
    typeof document === "undefined" ? "system" : preference(document.documentElement.dataset.theme),
  );

  useEffect(() => {
    function refresh() {
      try {
        const saved = preference(localStorage.getItem(storageKey));
        applyTheme(saved);
        setTheme(saved);
      } catch {
        // The current page still supports manual switching without storage.
      }
    }
    function onStorage(event: StorageEvent) {
      if (event.key === storageKey || event.key === null) refresh();
    }
    function onPageShow(event: PageTransitionEvent) {
      if (event.persisted) refresh();
    }
    window.addEventListener("storage", onStorage);
    window.addEventListener("pageshow", onPageShow);
    refresh();
    return () => {
      window.removeEventListener("storage", onStorage);
      window.removeEventListener("pageshow", onPageShow);
    };
  }, []);

  function change(value: string) {
    const next = preference(value);
    applyTheme(next);
    setTheme(next);
    try {
      if (next === "system") localStorage.removeItem(storageKey);
      else localStorage.setItem(storageKey, next);
    } catch {
      // Keep the selection active for this page.
    }
  }

  return (
    <div className="theme-select language-select">
      <SunMoon size={15} aria-hidden="true" />
      <select
        aria-label={c.theme.label}
        title={c.theme.label}
        value={theme}
        onChange={(event) => change(event.target.value)}
      >
        <option value="system">{c.theme.system}</option>
        <option value="light">{c.theme.light}</option>
        <option value="dark">{c.theme.dark}</option>
      </select>
    </div>
  );
}
