import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { en, zh, type PolicyBlock } from "../constants/copy";
import { pageDocuments, privacySectionIds, routes } from "../constants/routes";
import { staticLanguage } from "../lib/language";
import { render } from "../prerender";
import { formatPolicyDate, PrivacyPage } from "./privacy";

const shape = (block: PolicyBlock) =>
  block.kind === "paragraph"
    ? "paragraph"
    : block.kind === "list"
      ? `list:${block.items.map((item) => typeof item).join(",")}`
      : `table:${block.rows.map((row) => row.name).join(",")}`;

describe("privacy policy", () => {
  it("keeps the same structure in both languages", () => {
    for (const id of privacySectionIds) {
      expect(en.privacy.sections[id].blocks.map(shape)).toEqual(
        zh.privacy.sections[id].blocks.map(shape),
      );
    }
    expect(en.privacy.summary).toHaveLength(zh.privacy.summary.length);
  });

  it("lists exactly the extension's declared permissions", () => {
    const table = zh.privacy.sections.permissions.blocks.find(
      (block) => block.kind === "table",
    );
    expect(table?.kind === "table" && table.rows.map((row) => row.name)).toEqual(
      ["nativeMessaging", "activeTab", "scripting", "storage", "alarms", "optional_host_permissions", "clipboardWrite"],
    );
  });

  it.each([
    ["zh-CN", zh],
    ["en", en],
  ] as const)("renders every section with a table of contents (%s)", (language, c) => {
    const html = renderToStaticMarkup(
      <PrivacyPage c={c} control={staticLanguage} language={language} />,
    );
    for (const id of privacySectionIds) {
      expect(html).toContain(`id="${id}"`);
      expect(html).toContain(`href="#${id}"`);
    }
    expect(html).toContain(`<h1 id="policy-title">${c.privacy.title}</h1>`);
    expect(html).toContain('dateTime="2026-10-28"');
    expect(html).toContain('href="mailto:knothhe@gmail.com"');
  });

  it("formats the policy date without shifting it across time zones", () => {
    expect(formatPolicyDate("2026-10-23", "en")).toBe("October 23, 2026");
    expect(formatPolicyDate("2026-10-23", "zh-CN")).toBe("2026年10月23日");
  });
});

describe("site routes", () => {
  it("prerenders readable content for every static document", () => {
    expect(Object.keys(pageDocuments)).toEqual(["home", "privacy", "notFound"]);
    expect(render("privacy")).toContain(zh.privacy.sections.limits.title);
    expect(render("home")).toContain(zh.downloads.title);
    expect(render("notFound")).toContain(zh.notFound.title);
  });

  it("links every page to the policy and back to home sections", () => {
    for (const page of ["home", "privacy", "notFound"] as const) {
      const html = render(page);
      expect(html).toContain(`href="${routes.privacy}"`);
      expect(html).toContain('href="/#features"');
    }
  });
});
