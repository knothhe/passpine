import assert from "node:assert/strict";
import fs from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { test } from "node:test";
import { parseRelease } from "../src/lib/downloads.ts";
import { releaseSnapshot, syncRelease } from "./sync-release.mjs";

const raw = JSON.parse(
  await fs.readFile(
    new URL("../src/data/release.json", import.meta.url),
    "utf8",
  ),
);

test("saved release contains only assets accepted by the website validator", () => {
  const release = parseRelease(raw);
  assert.equal(release.assets.length, raw.assets.length);
  assert.deepEqual(releaseSnapshot(release), raw);
  assert.ok(release.assets.every((asset) => asset.platform || asset.browser));
});

test("refresh atomically replaces the saved release after successful validation", async () => {
  const directory = await fs.mkdtemp(
    path.join(os.tmpdir(), "passpine-release-sync-"),
  );
  const destination = path.join(directory, "release.json");
  try {
    await fs.writeFile(destination, "previous snapshot\n");
    const release = parseRelease(raw);
    const result = await syncRelease({
      destination,
      loadRelease: async () => release,
    });
    assert.equal(result.tag, release.tag);
    assert.equal(result.count, release.assets.length);
    assert.deepEqual(JSON.parse(await fs.readFile(destination, "utf8")), raw);
    assert.deepEqual(await fs.readdir(directory), ["release.json"]);
  } finally {
    await fs.rm(directory, { recursive: true, force: true });
  }
});

test("network errors abort refresh and preserve the previous saved release", async () => {
  const directory = await fs.mkdtemp(
    path.join(os.tmpdir(), "passpine-release-sync-"),
  );
  const destination = path.join(directory, "release.json");
  try {
    await fs.writeFile(destination, "previous snapshot\n");
    await assert.rejects(
      syncRelease({
        destination,
        loadRelease: async () => {
          throw new Error("API rate limited");
        },
      }),
      /API rate limited/,
    );
    assert.equal(await fs.readFile(destination, "utf8"), "previous snapshot\n");
    assert.deepEqual(await fs.readdir(directory), ["release.json"]);
  } finally {
    await fs.rm(directory, { recursive: true, force: true });
  }
});

test("unsafe release assets abort refresh and preserve the previous saved release", async () => {
  const directory = await fs.mkdtemp(
    path.join(os.tmpdir(), "passpine-release-sync-"),
  );
  const destination = path.join(directory, "release.json");
  try {
    await fs.writeFile(destination, "previous snapshot\n");
    const release = parseRelease(raw);
    release.assets[0].url = "https://untrusted.example/download.zip";
    await assert.rejects(
      syncRelease({ destination, loadRelease: async () => release }),
      /invalid assets/,
    );
    assert.equal(await fs.readFile(destination, "utf8"), "previous snapshot\n");
    assert.deepEqual(await fs.readdir(directory), ["release.json"]);
  } finally {
    await fs.rm(directory, { recursive: true, force: true });
  }
});
