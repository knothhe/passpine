import { readFileSync } from "node:fs";
import { runInNewContext } from "node:vm";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";
import { en, zh } from "../constants/copy";
import { pageDocuments, type PageId } from "../constants/routes";
import { staticLanguage } from "../lib/language";
import { render } from "../prerender";
import { SiteLayout } from "./site-chrome";

const site = new URL("../../", import.meta.url);
const read = (path: string) => readFileSync(new URL(path, site), "utf8");

function themeBeforePaint(getItem: (key: string) => string | null) {
  const dataset: { theme?: string } = {};
  const document = { documentElement: { dataset } };
  runInNewContext(read("public/theme.js"), { document, localStorage: { getItem } });
  return dataset.theme;
}

describe("website theme", () => {
  it.each(Object.keys(pageDocuments) as PageId[])("initializes the theme before painting %s", (page) => {
    const head = read(pageDocuments[page]).split("</head>")[0];
    const script = head.match(/<script\b[^>]*\bsrc="\/theme\.js"[^>]*>/)?.[0];
    expect(script).toBeDefined();
    expect(script).not.toMatch(/\b(?:async|defer|module)\b/);
    expect(render(page)).toContain(zh.displaySettings);
  });

  it.each(["light", "dark", "system", "invalid", null])("applies the saved %s preference", (saved) => {
    const getItem = vi.fn(() => saved);
    expect(themeBeforePaint(getItem)).toBe(saved === "light" || saved === "dark" ? saved : undefined);
    expect(getItem).toHaveBeenCalledWith("passpine-website-theme");
  });

  it("uses the system theme when storage is inaccessible", () => {
    expect(themeBeforePaint(() => { throw new Error("Storage unavailable"); })).toBeUndefined();
  });

  it.each([["en", en], ["zh-CN", zh]] as const)("labels both settings groups and their choices in %s", (_, c) => {
    const html = renderToStaticMarkup(<SiteLayout c={c} control={staticLanguage}>{null}</SiteLayout>);
    const header = html.split("</header>")[0];
    expect(header).toContain('aria-expanded="false" aria-controls="display-settings-panel"');
    expect(header).toContain(c.displaySettings);
    expect(header).toMatch(/id="display-settings-panel"[^>]*hidden/);
    expect(header).toContain(`<legend>${c.theme.label}</legend>`);
    expect(header).toContain(`<legend>${c.language.label}</legend>`);
    const labels = header.match(/<label>.*?<\/label>/g) ?? [];
    function checkChoice(group: string, value: string, text: string) {
      const label = labels.find((markup) =>
        markup.includes(`name="${group}"`) && markup.includes(`value="${value}"`),
      );
      expect(label).toContain(text);
      expect(label).toContain('type="radio"');
      expect(label?.includes('checked=""')).toBe(value === "system");
    }
    for (const value of ["system", "light", "dark"] as const) {
      checkChoice("site-appearance", value, c.theme[value]);
    }
    for (const value of ["system", "en", "zh-CN"] as const) {
      checkChoice("site-language", value, c.language[value === "zh-CN" ? "zh" : value]);
    }
  });
});
