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
    expect(render(page)).toContain(`aria-label="${zh.theme.label}"`);
  });

  it.each(["light", "dark", "system", "invalid", null])("applies the saved %s preference", (saved) => {
    const getItem = vi.fn(() => saved);
    expect(themeBeforePaint(getItem)).toBe(saved === "light" || saved === "dark" ? saved : undefined);
    expect(getItem).toHaveBeenCalledWith("passpine-website-theme");
  });

  it("uses the system theme when storage is inaccessible", () => {
    expect(themeBeforePaint(() => { throw new Error("Storage unavailable"); })).toBeUndefined();
  });

  it.each([["en", en], ["zh-CN", zh]] as const)("labels the header theme selector and its choices in %s", (_, c) => {
    const html = renderToStaticMarkup(<SiteLayout c={c} control={staticLanguage}>{null}</SiteLayout>);
    const selects = html.split("</header>")[0].match(/<select\b[^>]*>[\s\S]*?<\/select>/g);
    const themeSelect = selects?.find((select) => select.includes(`aria-label="${c.theme.label}"`));
    for (const value of ["system", "light", "dark"] as const) {
      expect(themeSelect).toMatch(new RegExp(`<option value="${value}"[^>]*>${c.theme[value]}</option>`));
    }
  });
});
