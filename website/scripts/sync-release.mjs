import { randomUUID } from "node:crypto";
import fs from "node:fs/promises";
import path from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";
import { fetchLatestRelease, parseRelease } from "../src/lib/downloads.ts";

const snapshotPath = fileURLToPath(
  new URL("../src/data/release.json", import.meta.url),
);

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
  loadRelease = fetchLatestRelease,
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
