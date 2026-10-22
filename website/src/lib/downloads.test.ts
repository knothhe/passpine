import { afterEach, describe, expect, it, vi } from "vitest";
import {
  chooseAsset,
  chooseExtension,
  detectDevice,
  detectPlatform,
  fetchLatestRelease,
  GITHUB_URL,
  LATEST_RELEASE_URL,
  RELEASE_API_URL,
  ReleaseError,
} from "./downloads";
import snapshot from "../data/release.json";
import { bundledRelease } from "./release-snapshot";

const asset = (name: string, overrides: Record<string, unknown> = {}) => ({
  name,
  size: 8192,
  state: "uploaded",
  browser_download_url: `${GITHUB_URL}/releases/download/v0.1.2/${name}`,
  ...overrides,
});

const release = (overrides: Record<string, unknown> = {}) => ({
  tag_name: "v0.1.2",
  html_url: `${GITHUB_URL}/releases/tag/v0.1.2`,
  draft: false,
  prerelease: false,
  published_at: "2026-10-22T09:30:18Z",
  assets: [
    asset("Passpine-0.1.2-macos-arm64.zip"),
    asset("Passpine-0.1.2-windows-amd64-setup.exe"),
    asset("Passpine-0.1.2-linux-amd64.tar.gz"),
    asset("passpine-0.1.2-chrome.zip"),
    asset("passpine-0.1.2-firefox.zip"),
  ],
  ...overrides,
});

function mockResponse(data: unknown = release(), status = 200) {
  return vi.stubGlobal(
    "fetch",
    vi.fn().mockResolvedValue(new Response(JSON.stringify(data), { status })),
  );
}

afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
  vi.useRealTimers();
});

describe("device detection", () => {
  it.each([
    [
      { userAgent: "Mozilla/5.0 (Windows NT 10.0; Win64; x64)" },
      { platform: "windows", architecture: "unknown" },
    ],
    [{ platform: "Linux x86_64" }, { platform: "linux", architecture: "unknown" }],
    [
      { userAgent: "Mozilla/5.0 (X11; Linux aarch64)" },
      { platform: "linux", architecture: "arm64" },
    ],
    [{ platform: "Win32" }, { platform: "windows", architecture: "unknown" }],
    [
      {
        userAgent: "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)",
        platform: "MacIntel",
      },
      { platform: "macos", architecture: "unknown" },
    ],
    [
      {
        userAgentData: {
          platform: "macOS",
          architecture: "arm",
          bitness: "64",
        },
      },
      { platform: "macos", architecture: "arm64" },
    ],
    [
      {
        userAgentData: {
          platform: "Windows",
          architecture: "x86",
          bitness: "64",
        },
      },
      { platform: "windows", architecture: "x64" },
    ],
    [
      {
        userAgent: "Windows NT 10.0; Win64; x64",
        userAgentData: {
          platform: "Windows",
          architecture: "arm",
          bitness: "64",
        },
      },
      { platform: "windows", architecture: "arm64" },
    ],
    [
      {
        userAgent: "Windows NT 10.0; Win64; x64",
        userAgentData: {
          platform: "Windows",
          architecture: "x86",
          bitness: "32",
        },
      },
      { platform: "windows", architecture: "unknown" },
    ],
    [
      { userAgent: "Mozilla/5.0 (Linux; Android 14; arm64)" },
      { platform: null, architecture: "unknown" },
    ],
    [
      { userAgent: "Mozilla/5.0 (iPhone; CPU iPhone OS 18_0)" },
      { platform: null, architecture: "unknown" },
    ],
    [
      {
        userAgent: "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)",
        platform: "MacIntel",
        maxTouchPoints: 5,
      },
      { platform: null, architecture: "unknown" },
    ],
    [
      {
        userAgentData: {
          platform: "Linux",
          mobile: true,
          architecture: "arm64",
        },
      },
      { platform: null, architecture: "unknown" },
    ],
    [
      { userAgent: "Mozilla/5.0 (X11; CrOS x86_64 14541.0.0)" },
      { platform: null, architecture: "unknown" },
    ],
    [{}, { platform: null, architecture: "unknown" }],
  ])("conservatively identifies %j", (input, expected) => {
    expect(detectPlatform(input)).toEqual(expected);
  });

  it.each([
    {
      platform: "linux" as const,
      info: {
        platform: "Linux x86_64",
        userAgent: "Mozilla/5.0 (X11; Linux x86_64; rv:145.0) Gecko/20100101 Firefox/145.0",
      },
    },
    {
      platform: "windows" as const,
      info: {
        platform: "Win32",
        userAgent: "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/145.0.0.0 Safari/537.36",
      },
    },
    {
      platform: "windows" as const,
      info: { platform: "Win32", userAgent: "Mozilla/5.0 (Windows NT 10.0; WOW64)" },
    },
  ])("does not recommend x64 for frozen $platform identifiers without architecture hints", async ({ platform, info }) => {
    const expected = { platform, architecture: "unknown" };
    expect(detectPlatform(info)).toEqual(expected);
    const device = await detectDevice(info);
    expect(device).toEqual(expected);
    expect(chooseAsset(bundledRelease, device.platform, device.architecture)).toBeNull();
    expect(await detectDevice({
      ...info,
      userAgentData: { getHighEntropyValues: vi.fn().mockRejectedValue(new Error("denied")) },
    })).toEqual(expected);
  });

  it.each([
    { platform: "linux" as const, identifier: "Linux x86_64", hint: "Linux" },
    { platform: "windows" as const, identifier: "Win32", hint: "Windows" },
  ])("recommends an available x64 $platform build after explicit 64-bit x86 Client Hints", async ({ platform, identifier, hint }) => {
    const device = await detectDevice({
      platform: identifier,
      userAgentData: {
        platform: hint,
        getHighEntropyValues: vi.fn().mockResolvedValue({ architecture: "x86", bitness: "64" }),
      },
    });
    expect(device).toEqual({ platform, architecture: "x64" });
    expect(chooseAsset(bundledRelease, device.platform, device.architecture)?.architecture).toBe("x64");
  });

  it("uses high entropy hints to distinguish Apple Silicon from a frozen Intel UA", async () => {
    const getHighEntropyValues = vi
      .fn()
      .mockResolvedValue({
        architecture: "arm",
        bitness: "64",
        platform: "macOS",
      });
    expect(
      await detectDevice({
        userAgent: "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)",
        userAgentData: { platform: "macOS", getHighEntropyValues },
      }),
    ).toEqual({ platform: "macos", architecture: "arm64" });
    expect(getHighEntropyValues).toHaveBeenCalledWith([
      "architecture",
      "bitness",
      "platform",
    ]);
  });

  it("remains usable when high entropy hints are denied", async () => {
    expect(
      await detectDevice({
        platform: "MacIntel",
        userAgentData: {
          getHighEntropyValues: vi.fn().mockRejectedValue(new Error("denied")),
        },
      }),
    ).toEqual({ platform: "macos", architecture: "unknown" });
  });

  it("preserves browser properties inherited from Navigator prototypes", async () => {
    const info = Object.create({
      platform: "Linux x86_64",
      userAgentData: Object.create({
        mobile: true,
        platform: "Linux",
        getHighEntropyValues: vi
          .fn()
          .mockResolvedValue({ architecture: "arm", bitness: "64" }),
      }),
    });
    expect(await detectDevice(info)).toEqual({
      platform: null,
      architecture: "unknown",
    });
  });
});

describe("published release assets", () => {
  it("bundles only verified published assets for API failure fallback", () => {
    expect(bundledRelease.assets).toHaveLength(snapshot.assets.length);
    expect(bundledRelease.assets.length).toBeGreaterThan(0);
    expect(bundledRelease.assets.every(asset => asset.platform || asset.browser)).toBe(true);
    expect(bundledRelease.assets.every(asset => asset.url.startsWith(`${GITHUB_URL}/releases/download/${bundledRelease.tag}/`))).toBe(true);
  });
  it("loads the live release naming scheme and matches each operating system", async () => {
    mockResponse();
    const result = await fetchLatestRelease();
    expect(result).toMatchObject({
      tag: "v0.1.2",
      version: "0.1.2",
      url: `${GITHUB_URL}/releases/tag/v0.1.2`,
    });
    expect(result.assets).toHaveLength(5);
    expect(chooseAsset(result, "macos", "arm64")?.name).toBe(
      "Passpine-0.1.2-macos-arm64.zip",
    );
    expect(chooseAsset(result, "windows", "x64")?.name).toBe(
      "Passpine-0.1.2-windows-amd64-setup.exe",
    );
    expect(chooseAsset(result, "linux", "x64")?.name).toBe(
      "Passpine-0.1.2-linux-amd64.tar.gz",
    );
    expect(fetch).toHaveBeenCalledWith(
      RELEASE_API_URL,
      expect.objectContaining({
        credentials: "omit",
        redirect: "error",
        headers: { Accept: "application/vnd.github+json" },
      }),
    );
  });

  it("does not send Intel Macs, unknown Macs or ARM Windows to incompatible assets", async () => {
    mockResponse();
    const result = await fetchLatestRelease();
    expect(chooseAsset(result, "macos", "x64")).toBeNull();
    expect(chooseAsset(result, "macos", "unknown")).toBeNull();
    expect(chooseAsset(result, "windows", "arm64")).toBeNull();
    expect(chooseAsset(result, "linux", "arm64")).toBeNull();
    expect(chooseAsset(result, null, "x64")).toBeNull();
    expect(chooseAsset(null, "windows", "x64")).toBeNull();
  });

  it("matches a future Intel Mac build without selecting the ARM build", async () => {
    mockResponse(
      release({
        assets: [
          asset("Passpine-0.1.2-macos-arm64.zip"),
          asset("Passpine-0.1.2-macos-x86_64.zip"),
        ],
      }),
    );
    const result = await fetchLatestRelease();
    expect(chooseAsset(result, "macos", "x64")?.name).toBe(
      "Passpine-0.1.2-macos-x86_64.zip",
    );
  });

  it("selects browser packages separately, with Edge using the Chrome build", async () => {
    mockResponse();
    const result = await fetchLatestRelease();
    expect(chooseExtension(result, "chrome")?.name).toBe(
      "passpine-0.1.2-chrome.zip",
    );
    expect(chooseExtension(result, "firefox")?.name).toBe(
      "passpine-0.1.2-firefox.zip",
    );
    expect(chooseExtension(result, "edge")).toEqual(
      chooseExtension(result, "chrome"),
    );
    expect(chooseExtension(null, "chrome")).toBeNull();
  });

  it.each([
    {
      browser_download_url:
        "https://evil.example/Passpine-0.1.2-macos-arm64.zip",
    },
    {
      browser_download_url:
        "https://github.com/evil/passpine/releases/download/v0.1.2/Passpine-0.1.2-macos-arm64.zip",
    },
    {
      browser_download_url: `${GITHUB_URL}/releases/download/v0.1.1/Passpine-0.1.2-macos-arm64.zip`,
    },
    {
      browser_download_url: `${GITHUB_URL}/releases/download/v0.1.2/Passpine-0.1.2-macos-arm64.zip?redirect=evil`,
    },
    {
      browser_download_url:
        "https://github.com.evil.example/knothhe/passpine/releases/download/v0.1.2/Passpine-0.1.2-macos-arm64.zip",
    },
    {
      browser_download_url:
        "https://evil@github.com/knothhe/passpine/releases/download/v0.1.2/Passpine-0.1.2-macos-arm64.zip",
    },
    { size: 0 },
    { size: -1 },
    { size: "8192" },
    { state: "new" },
  ])("discards unsafe or unfinished assets %j", async (overrides) => {
    mockResponse(
      release({
        assets: [
          asset("Passpine-0.1.2-macos-arm64.zip", overrides),
          asset("Passpine-0.1.2-linux-amd64.tar.gz"),
        ],
      }),
    );
    const result = await fetchLatestRelease();
    expect(result.assets).toHaveLength(1);
    expect(chooseAsset(result, "macos", "arm64")).toBeNull();
  });

  it("ignores source archives, a different version and mismatched package types", async () => {
    mockResponse(
      release({
        assets: [
          asset("source.zip"),
          asset("Passpine-0.1.1-macos-arm64.zip"),
          asset("Passpine-0.1.2-windows-amd64.zip"),
          asset("Passpine-0.1.2-linux-amd64-setup.exe"),
          asset("passpine-0.1.2-chrome.zip"),
        ],
      }),
    );
    const result = await fetchLatestRelease();
    expect(result.assets.map((item) => item.name)).toEqual([
      "passpine-0.1.2-chrome.zip",
    ]);
    expect(chooseAsset(result, "windows", "x64")).toBeNull();
  });

  it.each([
    null,
    [],
    {},
    release({ draft: true }),
    release({ prerelease: true }),
    release({ tag_name: "../evil" }),
    release({ assets: [] }),
    release({ html_url: "https://evil.example/release" }),
  ])("rejects invalid release metadata %j", async (data) => {
    mockResponse(data);
    await expect(fetchLatestRelease()).rejects.toMatchObject({
      code: "invalid",
    });
  });
});

describe("release API failure handling", () => {
  it("exposes a stable GitHub fallback URL independently of the API", () => {
    expect(LATEST_RELEASE_URL).toBe(
      "https://github.com/knothhe/passpine/releases/latest",
    );
  });

  it("handles GitHub API rate limiting", async () => {
    mockResponse({ message: "API rate limit exceeded" }, 403);
    await expect(fetchLatestRelease()).rejects.toEqual(
      new ReleaseError("http"),
    );
  });

  it("handles offline fetch failures", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockRejectedValue(new TypeError("Failed to fetch")),
    );
    await expect(fetchLatestRelease()).rejects.toMatchObject({
      code: "network",
    });
  });

  it("handles malformed API JSON", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response("not json")));
    await expect(fetchLatestRelease()).rejects.toMatchObject({
      code: "invalid",
    });
  });

  it("does not fetch when the caller already aborted", async () => {
    mockResponse();
    const controller = new AbortController();
    controller.abort();
    await expect(fetchLatestRelease(controller.signal)).rejects.toMatchObject({
      code: "aborted",
    });
    expect(fetch).not.toHaveBeenCalled();
  });

  it("aborts a pending request on caller cancellation", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(
        (_url: string, init: RequestInit) =>
          new Promise((_resolve, reject) => {
            init.signal?.addEventListener("abort", () =>
              reject(new DOMException("Aborted", "AbortError")),
            );
          }),
      ),
    );
    const controller = new AbortController();
    const request = fetchLatestRelease(controller.signal);
    const assertion = expect(request).rejects.toMatchObject({
      code: "aborted",
    });
    controller.abort();
    await assertion;
  });

  it("times out stalled requests so the fallback can be shown", async () => {
    vi.useFakeTimers();
    vi.stubGlobal(
      "fetch",
      vi.fn(
        (_url: string, init: RequestInit) =>
          new Promise((_resolve, reject) => {
            init.signal?.addEventListener("abort", () =>
              reject(new DOMException("Aborted", "AbortError")),
            );
          }),
      ),
    );
    const request = fetchLatestRelease();
    const assertion = expect(request).rejects.toMatchObject({
      code: "timeout",
    });
    await vi.advanceTimersByTimeAsync(8_000);
    await assertion;
  });
});
