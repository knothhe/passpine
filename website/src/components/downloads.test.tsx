import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { DownloadSection } from "../pages/home";
import { en, zh } from "../constants/copy";
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
    expect(html).not.toContain(zh.downloads.extensionTitle);
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
    expect(html).toContain(en.downloads.latest(bundledRelease.tag));
    expect(html).toContain('aria-label="Download for Windows · x64"');
    expect(html).toContain(
      en.downloads.extensionDownload(en.downloads.chromium),
    );
    expect(html).not.toContain(zh.downloads.installGuide);
    expect(html).not.toContain(zh.downloads.extensionTitle);
  });
});
