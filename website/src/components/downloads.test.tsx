import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { DownloadSection } from "../pages/home";
import { chromeWebStoreUrl } from "../constants/routes";
import { en, homebrewInstallCommand, linuxInstallCommand, zh } from "../constants/copy";
import { bundledRelease } from "../lib/release-snapshot";
import type { Device, Release } from "../lib/downloads";

const device: Device = { platform: "windows", architecture: "x64" };
function render(
  release: Release,
  copy = zh,
  status: "ready" | "error" = "ready",
) {
  return renderToStaticMarkup(
    <DownloadSection
      c={copy}
      device={device}
      releaseState={{ release, status, retry: () => {} }}
    />,
  );
}

describe("available website downloads", () => {
  it.each([["en", en], ["zh-CN", zh]] as const)("shows copyable install commands on supported cards (%s)", (_, copy) => {
    const cards = render(bundledRelease, copy).match(/<article\b[\s\S]*?<\/article>/g) ?? [];
    const heading = `<h4>${copy.downloads.installCommand}</h4>`;
    const copyButton = `${copy.downloads.copyCommand}</button>`;
    for (const [platform, command, note] of [
      ["macos", homebrewInstallCommand, copy.downloads.homebrewNote],
      ["linux", linuxInstallCommand, copy.downloads.linuxCommandNote],
      ["windows", null, null],
    ] as const) {
      const card = cards.find((part) => part.includes(`>${copy.downloads.platforms[platform]}</h3>`));
      expect(card).toContain(copy.downloads.notes[platform]);
      if (command) {
        expect(card).toContain(heading);
        expect(card).toContain(`<code>${command}</code>`);
        expect(card).toContain(copyButton);
        expect(card).toContain(note);
        expect(card).not.toContain("<details");
      } else {
        expect(card).not.toContain(heading);
        expect(card).not.toContain(copyButton);
        expect(card).not.toContain("<code>");
      }
    }
  });

  it("omits the Linux installer command when only another Linux architecture is available", () => {
    const assets = bundledRelease.assets.map((asset) =>
      asset.platform === "linux" ? { ...asset, architecture: "arm64" as const } : asset,
    );
    const html = render({ ...bundledRelease, assets });
    expect(html).toContain(">Linux</h3>");
    expect(html).not.toContain(linuxInstallCommand);
  });

  it("renders only released platforms, architectures, and browser packages", () => {
    const release = {
      ...bundledRelease,
      assets: bundledRelease.assets.filter(
        (asset) => asset.platform === "macos",
      ),
    };
    const html = render(release);
    expect(html).toContain(">macOS</h3>");
    expect(html).toContain("Apple Silicon");
    expect(html).not.toContain(">Windows</h3>");
    expect(html).not.toContain(">Linux</h3>");
    expect(html).not.toContain("Intel");
    expect(html).toContain(`href="${chromeWebStoreUrl}"`);
    expect(html).toContain(zh.downloads.chromeStore);
    expect(html).not.toContain(zh.downloads.extensionDownload(zh.downloads.chromium));
    expect(html).not.toContain(zh.downloads.extensionDownload(zh.downloads.firefox));
    for (const asset of release.assets)
      expect(html).toContain(`href="${asset.url}"`);
  });

  it("keeps actual download URLs available when the latest API request fails", () => {
    const html = render(bundledRelease, zh, "error");
    expect(html).toContain(zh.downloads.savedVersion(bundledRelease.tag));
    expect(html).toContain(zh.downloads.error);
    expect(html).not.toContain(zh.downloads.latest(bundledRelease.tag));
    for (const asset of bundledRelease.assets)
      expect(html).toContain(`href="${asset.url}"`);
  });

  it("orders the detected system first and labels its architecture explicitly", () => {
    const html = render(bundledRelease);
    expect(html.indexOf(">Windows</h3>")).toBeLessThan(
      html.indexOf(">macOS</h3>"),
    );
    expect(html).toContain(zh.downloads.currentSystem);
    expect(html).toContain('aria-label="下载 Windows · x64"');
    expect(html).not.toContain("Intel");
  });

  it("translates download guidance and accessible link names into English", () => {
    const html = render(bundledRelease, en);
    expect(html).toContain(en.downloads.chromeStore);
    expect(html).toContain(en.downloads.latest(bundledRelease.tag));
    expect(html).toContain('aria-label="Download for Windows · x64"');
    expect(html).toContain(
      en.downloads.extensionDownload(en.downloads.chromium),
    );
    expect(html).not.toContain(zh.downloads.installGuide);
    expect(html).not.toContain(zh.downloads.extensionTitle);
    expect(html).not.toContain(zh.downloads.chromeStore);
  });
});
