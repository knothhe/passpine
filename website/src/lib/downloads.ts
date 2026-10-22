export const GITHUB_URL = "https://github.com/knothhe/passpine";
export const RELEASES_URL = `${GITHUB_URL}/releases`;
export const LATEST_RELEASE_URL = `${RELEASES_URL}/latest`;
export const RELEASE_API_URL =
  "https://api.github.com/repos/knothhe/passpine/releases/latest";

export type Platform = "macos" | "windows" | "linux";
export type Architecture = "arm64" | "x64" | "unknown";
export type ExtensionBrowser = "chrome" | "firefox";

export interface Device {
  platform: Platform | null;
  architecture: Architecture;
}

interface ClientHints {
  platform?: string;
  mobile?: boolean;
  architecture?: string;
  bitness?: string;
  getHighEntropyValues?: (hints: string[]) => Promise<{
    architecture?: string;
    bitness?: string;
    platform?: string;
  }>;
}

export interface NavigatorInfo {
  userAgent?: string;
  platform?: string;
  maxTouchPoints?: number;
  userAgentData?: ClientHints;
}

export interface ReleaseAsset {
  name: string;
  url: string;
  size: number;
  platform: Platform | null;
  architecture: Exclude<Architecture, "unknown"> | null;
  browser: ExtensionBrowser | null;
}

export interface Release {
  tag: string;
  version: string;
  url: string;
  publishedAt: string | null;
  assets: ReleaseAsset[];
}

export type ReleaseErrorCode = "network" | "http" | "invalid" | "aborted" | "timeout";

export class ReleaseError extends Error {
  readonly code: ReleaseErrorCode;

  constructor(code: ReleaseErrorCode) {
    super(`Release request failed: ${code}`);
    this.name = "ReleaseError";
    this.code = code;
  }
}

function currentNavigator(): NavigatorInfo {
  return typeof navigator === "undefined" ? {} : navigator;
}

/** Reduced browser identifiers can identify the OS but cannot prove x64 architecture. */
export function detectPlatform(
  info: NavigatorInfo = currentNavigator(),
): Device {
  const userAgent = info.userAgent ?? "";
  const hints = info.userAgentData;
  const identifier = `${hints?.platform ?? ""} ${info.platform ?? ""} ${userAgent}`;
  const mobile =
    hints?.mobile ||
    /Android|iPhone|iPad|iPod|Windows Phone/i.test(identifier) ||
    (/Mac/i.test(info.platform ?? "") && (info.maxTouchPoints ?? 0) > 1);
  if (mobile) return { platform: null, architecture: "unknown" };

  let platform: Platform | null = null;
  if (/Windows|Win32|Win64/i.test(identifier)) platform = "windows";
  else if (/macOS|Macintosh|MacIntel|MacPPC|Mac OS X/i.test(identifier))
    platform = "macos";
  else if (/Linux/i.test(identifier) && !/CrOS/i.test(identifier))
    platform = "linux";
  if (!platform) return { platform: null, architecture: "unknown" };

  const hintedArchitecture = hints?.architecture?.toLowerCase();
  if (hints?.bitness === "32") return { platform, architecture: "unknown" };
  if (
    /^(arm64|aarch64)$/.test(hintedArchitecture ?? "") ||
    (hintedArchitecture === "arm" && hints?.bitness === "64")
  ) {
    return { platform, architecture: "arm64" };
  }
  if (
    /^(x86_64|amd64|x64)$/.test(hintedArchitecture ?? "") ||
    (hintedArchitecture === "x86" && hints?.bitness === "64")
  ) {
    return { platform, architecture: "x64" };
  }
  if (/arm64|aarch64/i.test(identifier))
    return { platform, architecture: "arm64" };

  // Browsers freeze MacIntel, Linux x86_64 and Win64 identifiers even on ARM64.
  // Only explicit architecture hints above can establish x64 compatibility.
  return { platform, architecture: "unknown" };
}

/** Prefer architecture Client Hints where the browser makes them available. */
export async function detectDevice(
  info: NavigatorInfo = currentNavigator(),
): Promise<Device> {
  const hints = info.userAgentData;
  if (!hints?.getHighEntropyValues) return detectPlatform(info);
  try {
    const values = await hints.getHighEntropyValues([
      "architecture",
      "bitness",
      "platform",
    ]);
    // Navigator and NavigatorUAData expose values on their prototypes, so do
    // not rely on object spread to copy their browser-provided properties.
    return detectPlatform({
      userAgent: info.userAgent,
      platform: info.platform,
      maxTouchPoints: info.maxTouchPoints,
      userAgentData: {
        platform: values.platform ?? hints.platform,
        mobile: hints.mobile,
        architecture: values.architecture ?? hints.architecture,
        bitness: values.bitness ?? hints.bitness,
      },
    });
  } catch {
    return detectPlatform(info);
  }
}

function object(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function escapeRegex(value: string): string {
  return value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

function parseAsset(
  value: unknown,
  tag: string,
  version: string,
): ReleaseAsset | null {
  if (
    !object(value) ||
    typeof value.name !== "string" ||
    typeof value.browser_download_url !== "string" ||
    !Number.isSafeInteger(value.size) ||
    (value.size as number) <= 0 ||
    value.state !== "uploaded"
  )
    return null;
  const name = value.name;
  const expectedUrl = `${GITHUB_URL}/releases/download/${encodeURIComponent(tag)}/${encodeURIComponent(name)}`;
  // Accept only the canonical URL for this repository, tag and asset name.
  // This also excludes credentials, ports, traversal, query strings and redirects to other hosts.
  if (value.browser_download_url !== expectedUrl) return null;

  const escapedVersion = escapeRegex(version);
  const extension = new RegExp(
    `^passpine-${escapedVersion}-(chrome|firefox)\\.zip$`,
  ).exec(name);
  if (extension) {
    return {
      name,
      url: expectedUrl,
      size: value.size as number,
      platform: null,
      architecture: null,
      browser: extension[1] as ExtensionBrowser,
    };
  }
  const desktop = new RegExp(
    `^Passpine-${escapedVersion}-(macos|windows|linux)-(arm64|aarch64|amd64|x86_64|x64)(\\.zip|-setup\\.exe|\\.tar\\.gz)$`,
  ).exec(name);
  if (!desktop) return null;
  const platform = desktop[1] as Platform;
  const expectedSuffix = {
    macos: ".zip",
    windows: "-setup.exe",
    linux: ".tar.gz",
  }[platform];
  if (desktop[3] !== expectedSuffix) return null;
  return {
    name,
    url: expectedUrl,
    size: value.size as number,
    platform,
    architecture: /^(arm64|aarch64)$/.test(desktop[2]!) ? "arm64" : "x64",
    browser: null,
  };
}

export function parseRelease(value: unknown): Release {
  if (
    !object(value) ||
    typeof value.tag_name !== "string" ||
    value.draft !== false ||
    value.prerelease !== false ||
    !Array.isArray(value.assets)
  )
    throw new ReleaseError("invalid");
  const match = /^v?(\d+\.\d+\.\d+)$/.exec(value.tag_name);
  if (!match) throw new ReleaseError("invalid");
  const version = match[1]!;
  const url = `${RELEASES_URL}/tag/${encodeURIComponent(value.tag_name)}`;
  if (value.html_url !== url) throw new ReleaseError("invalid");
  const assets = value.assets
    .map((asset) => parseAsset(asset, value.tag_name as string, version))
    .filter((asset): asset is ReleaseAsset => asset !== null);
  if (assets.length === 0) throw new ReleaseError("invalid");
  return {
    tag: value.tag_name,
    version,
    url,
    publishedAt:
      typeof value.published_at === "string" &&
      Number.isFinite(Date.parse(value.published_at))
        ? value.published_at
        : null,
    assets,
  };
}

/** Errors leave the UI free to offer LATEST_RELEASE_URL as a stable fallback. */
export async function fetchLatestRelease(
  signal?: AbortSignal,
): Promise<Release> {
  if (signal?.aborted) throw new ReleaseError("aborted");
  const controller = new AbortController();
  const abort = () => controller.abort();
  signal?.addEventListener("abort", abort, { once: true });
  let timedOut = false;
  const timer = setTimeout(() => {
    timedOut = true;
    controller.abort();
  }, 8_000);
  try {
    const response = await fetch(RELEASE_API_URL, {
      headers: { Accept: "application/vnd.github+json" },
      signal: controller.signal,
      credentials: "omit",
      redirect: "error",
    });
    if (!response.ok) throw new ReleaseError("http");
    let data: unknown;
    try {
      data = await response.json();
    } catch {
      throw new ReleaseError("invalid");
    }
    return parseRelease(data);
  } catch (error) {
    if (timedOut) throw new ReleaseError("timeout");
    if (signal?.aborted) throw new ReleaseError("aborted");
    if (error instanceof ReleaseError) throw error;
    throw new ReleaseError("network");
  } finally {
    clearTimeout(timer);
    signal?.removeEventListener("abort", abort);
  }
}

/** No cross-architecture fallback: an unavailable build must remain unavailable. */
export function chooseAsset(
  release: Release | null,
  platform: Platform | null,
  architecture: Architecture,
): ReleaseAsset | null {
  if (!release || !platform || architecture === "unknown") return null;
  return (
    release.assets.find(
      (asset) =>
        asset.platform === platform && asset.architecture === architecture,
    ) ?? null
  );
}

/** Edge uses the Chrome extension package. */
export function chooseExtension(
  release: Release | null,
  browser: ExtensionBrowser | "edge",
): ReleaseAsset | null {
  return (
    release?.assets.find(
      (asset) => asset.browser === (browser === "edge" ? "chrome" : browser),
    ) ?? null
  );
}
