import { useEffect, useRef, useState } from "react";
import { ChevronDown, Monitor, Moon, Settings2, Sun } from "lucide-react";
import type { LanguageControl } from "../lib/language";
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

function useThemePreference() {
  const [theme, setTheme] = useState<Theme>(() =>
    typeof document === "undefined"
      ? "system"
      : preference(document.documentElement.dataset.theme),
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

  function change(next: Theme) {
    applyTheme(next);
    setTheme(next);
    try {
      if (next === "system") localStorage.removeItem(storageKey);
      else localStorage.setItem(storageKey, next);
    } catch {
      // Keep the selection active for this page.
    }
  }

  return { theme, change };
}

const appearances = [
  { value: "system", Icon: Monitor },
  { value: "light", Icon: Sun },
  { value: "dark", Icon: Moon },
] as const;
const languages = ["system", "en", "zh-CN"] as const;

export function DisplaySettings({
  c,
  control,
}: {
  c: Copy;
  control: LanguageControl;
}) {
  const [open, setOpen] = useState(false);
  const root = useRef<HTMLDivElement>(null);
  const trigger = useRef<HTMLButtonElement>(null);
  const { theme, change } = useThemePreference();

  useEffect(() => {
    if (!open) return;
    root.current
      ?.querySelector<HTMLInputElement>("input:checked")
      ?.focus({ preventScroll: true });
    function dismissOutside(event: Event) {
      if (event.target instanceof Node && !root.current?.contains(event.target)) {
        setOpen(false);
      }
    }
    document.addEventListener("pointerdown", dismissOutside, true);
    document.addEventListener("focusin", dismissOutside, true);
    return () => {
      document.removeEventListener("pointerdown", dismissOutside, true);
      document.removeEventListener("focusin", dismissOutside, true);
    };
  }, [open]);

  return (
    <div
      ref={root}
      className="display-settings"
      onKeyDown={(event) => {
        if (open && event.key === "Escape") {
          event.stopPropagation();
          setOpen(false);
          trigger.current?.focus({ preventScroll: true });
        }
      }}
    >
      <button
        className="display-settings-trigger"
        type="button"
        ref={trigger}
        aria-expanded={open}
        aria-controls="display-settings-panel"
        onClick={() => setOpen((current) => !current)}
      >
        <Settings2 size={15} aria-hidden="true" />
        {c.displaySettings}
        <ChevronDown size={13} aria-hidden="true" />
      </button>
      <div id="display-settings-panel" className="display-settings-panel" hidden={!open}>
        <fieldset>
          <legend>{c.theme.label}</legend>
          <div className="appearance-options">
            {appearances.map(({ value, Icon }) => (
              <label key={value}>
                <input
                  type="radio"
                  name="site-appearance"
                  value={value}
                  checked={theme === value}
                  onChange={() => change(value)}
                />
                <span>
                  <Icon size={18} aria-hidden="true" />
                  {c.theme[value]}
                </span>
              </label>
            ))}
          </div>
        </fieldset>
        <fieldset>
          <legend>{c.language.label}</legend>
          <div className="language-options">
            {languages.map((value) => (
              <label key={value}>
                <span lang={value === "system" ? undefined : value}>
                  {c.language[value === "zh-CN" ? "zh" : value]}
                </span>
                <input
                  type="radio"
                  name="site-language"
                  value={value}
                  checked={control.preference === value}
                  onChange={() => control.setPreference(value)}
                />
              </label>
            ))}
          </div>
        </fieldset>
      </div>
    </div>
  );
}
