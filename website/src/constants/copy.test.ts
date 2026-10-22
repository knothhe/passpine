import { describe, expect, it } from "vitest";
import { resolveLanguage } from "./copy";

describe("website language selection", () => {
  it.each([
    ["zh", "zh-CN"],
    ["zh-CN", "zh-CN"],
    ["zh-SG", "zh-CN"],
    ["zh-Hans", "zh-CN"],
    ["zh-Hans-TW", "zh-CN"],
    [" ZH_cn.UTF-8 ", "zh-CN"],
    ["zh_CN@variant", "zh-CN"],
    ["zh-TW", "en"],
    ["zh-HK", "en"],
    ["zh-MO", "en"],
    ["zh-Hant", "en"],
    ["zh-Hant-CN", "en"],
    ["zh-Latn-CN", "en"],
    ["en-US", "en"],
    ["ja-JP", "en"],
    ["", "en"],
  ] as const)("matches the product fallback for %s", (locale, expected) => {
    expect(resolveLanguage("system", [locale])).toBe(expected);
  });

  it("uses only the preferred system language, including unsupported variants", () => {
    expect(resolveLanguage("system", ["zh-TW", "zh-CN", "en"])).toBe("en");
    expect(resolveLanguage("system", ["ja-JP", "zh-CN"])).toBe("en");
    expect(resolveLanguage("system", [])).toBe("en");
  });

  it("keeps explicit preferences regardless of the system language", () => {
    expect(resolveLanguage("en", ["zh-CN"])).toBe("en");
    expect(resolveLanguage("zh-CN", ["zh-Hant-TW"])).toBe("zh-CN");
    expect(resolveLanguage("zh-CN", [])).toBe("zh-CN");
  });
});
