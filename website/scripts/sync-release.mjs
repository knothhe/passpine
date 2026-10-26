import { randomUUID } from "node:crypto";
import { execFile } from "node:child_process";
import fs from "node:fs/promises";
import path from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";
import { promisify } from "node:util";
import { RELEASE_API_URL, parseRelease } from "../src/lib/downloads.ts";

const runFile = promisify(execFile);

const snapshotPath = fileURLToPath(
  new URL("../src/data/release.json", import.meta.url),
);

function httpError(response, authenticated) {
  const details = [`GitHub Release request failed: HTTP ${response.status}.`];
  const retry = response.headers.get("retry-after");
  if (response.status === 429 || response.headers.get("x-ratelimit-remaining") === "0"
    || (response.status === 403 && retry)) {
    details.push("GitHub API rate limit exceeded.");
    const reset = Number(response.headers.get("x-ratelimit-reset"));
    const date = new Date(reset * 1000);
    if (reset > 0 && Number.isFinite(date.getTime())) {
      details.push(`Rate limit resets at ${date.toISOString()}.`);
    }
    if (retry && /^\d+$/.test(retry)) details.push(`Retry after ${retry} seconds.`);
  }
  if ([401, 403, 429].includes(response.status)) {
    details.push(authenticated
      ? "Check GH_TOKEN/GITHUB_TOKEN and repository access."
      : "Run gh auth login --hostname github.com, or set GH_TOKEN/GITHUB_TOKEN for deployment.");
  }
  return new Error(details.join(" "));
}

/** Deployment credentials stay in Node; the website keeps using anonymous fetch. */
export async function fetchReleaseForSync({
  env = process.env,
  fetchImpl = fetch,
  execFileImpl = runFile,
} = {}) {
  const token = env.GH_TOKEN?.trim() || env.GITHUB_TOKEN?.trim();
  if (!token) {
    let output;
    try {
      const result = await execFileImpl("gh", [
        "api", "--hostname", "github.com",
        "repos/knothhe/passpine/releases/latest",
        "--header", "Accept: application/vnd.github+json",
      ], {
        encoding: "utf8",
        timeout: 10_000,
        maxBuffer: 2 * 1024 * 1024,
        windowsHide: true,
        env: { ...env, GH_PROMPT_DISABLED: "1" },
      });
      output = result.stdout;
    } catch {
      // gh is optional. A missing CLI, login or failed request can still use
      // the public API; never forward CLI output that could contain secrets.
    }
    if (output !== undefined) {
      let data;
      try {
        data = JSON.parse(output);
      } catch {
        throw new Error("GitHub CLI returned invalid release JSON.");
      }
      return parseRelease(data);
    }
  }

  let response;
  try {
    response = await fetchImpl(RELEASE_API_URL, {
      headers: {
        Accept: "application/vnd.github+json",
        "User-Agent": "passpine-website-release-sync",
        ...(token ? { Authorization: `Bearer ${token}` } : {}),
      },
      signal: AbortSignal.timeout(10_000),
      credentials: "omit",
      redirect: "error",
    });
  } catch (error) {
    const reason = ["TimeoutError", "AbortError"].includes(error?.name)
      ? "request timed out after 10 seconds"
      : "network error";
    throw new Error(`GitHub Release request failed: ${reason}.`);
  }
  if (!response.ok) throw httpError(response, Boolean(token));
  let data;
  try {
    data = await response.json();
  } catch {
    throw new Error("GitHub Release response was not valid JSON.");
  }
  return parseRelease(data);
}

/** Save only canonical metadata accepted by the same validator the website uses. */
export function releaseSnapshot(release) {
  const snapshot = {
    tag_name: release.tag,
    html_url: release.url,
    draft: false,
    prerelease: false,
    published_at: release.publishedAt,
    assets: release.assets.map((asset) => ({
      name: asset.name,
      size: asset.size,
      state: "uploaded",
      browser_download_url: asset.url,
    })),
  };
  const validated = parseRelease(snapshot);
  if (validated.assets.length !== release.assets.length) {
    throw new Error("Release snapshot contains invalid assets.");
  }
  return snapshot;
}

/** Fetch and validate first. A failed refresh must leave the saved release untouched. */
export async function syncRelease({
  destination = snapshotPath,
  loadRelease = fetchReleaseForSync,
} = {}) {
  const release = await loadRelease();
  const snapshot = releaseSnapshot(release);
  await fs.mkdir(path.dirname(destination), { recursive: true });
  const temporary = `${destination}.${randomUUID()}.tmp`;
  try {
    await fs.writeFile(temporary, `${JSON.stringify(snapshot, null, 2)}\n`, {
      flag: "wx",
    });
    await fs.rename(temporary, destination);
  } finally {
    await fs.rm(temporary, { force: true });
  }
  return { tag: release.tag, count: snapshot.assets.length, destination };
}

if (
  process.argv[1] &&
  pathToFileURL(path.resolve(process.argv[1])).href === import.meta.url
) {
  try {
    const result = await syncRelease();
    console.log(
      `Saved ${result.tag}: ${result.count} verified release assets.`,
    );
  } catch (error) {
    console.error(
      `Release synchronization failed. Saved metadata was not changed. ${error.message}`,
    );
    process.exitCode = 1;
  }
}
