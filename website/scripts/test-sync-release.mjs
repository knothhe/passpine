import assert from "node:assert/strict";
import fs from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { test } from "node:test";
import { RELEASE_API_URL, parseRelease } from "../src/lib/downloads.ts";
import {
  fetchReleaseForSync,
  releaseSnapshot,
  syncRelease,
} from "./sync-release.mjs";

const raw = JSON.parse(
  await fs.readFile(
    new URL("../src/data/release.json", import.meta.url),
    "utf8",
  ),
);

test("deployment uses GH_TOKEN before GITHUB_TOKEN without invoking gh", async () => {
  for (const [env, expectedToken] of [
    [{ GH_TOKEN: "test-gh-token", GITHUB_TOKEN: "test-github-token" }, "test-gh-token"],
    [{ GITHUB_TOKEN: "test-github-token" }, "test-github-token"],
  ]) {
    let ghCalls = 0;
    let requests = 0;
    const release = await fetchReleaseForSync({
      env,
      execFileImpl: async () => {
        ghCalls++;
        return { stdout: JSON.stringify(raw) };
      },
      fetchImpl: async (url, options) => {
        requests++;
        assert.equal(url, RELEASE_API_URL);
        assert.equal(options.headers.Authorization, `Bearer ${expectedToken}`);
        assert.equal(options.redirect, "error");
        assert.equal(options.credentials, "omit");
        assert.ok(options.signal instanceof AbortSignal);
        return Response.json(raw);
      },
    });
    assert.equal(ghCalls, 0);
    assert.equal(requests, 1);
    assert.deepEqual(release, parseRelease(raw));
  }
});

test("deployment reuses gh authentication for github.com without an anonymous request", async () => {
  let requests = 0;
  const release = await fetchReleaseForSync({
    env: { GH_HOST: "other.example" },
    execFileImpl: async (command, args, options) => {
      assert.equal(command, "gh");
      assert.deepEqual(args, [
        "api", "--hostname", "github.com",
        "repos/knothhe/passpine/releases/latest",
        "--header", "Accept: application/vnd.github+json",
      ]);
      assert.equal(options.env.GH_PROMPT_DISABLED, "1");
      assert.equal(options.timeout, 10_000);
      assert.equal(options.windowsHide, true);
      return { stdout: JSON.stringify(raw) };
    },
    fetchImpl: async () => {
      requests++;
      return Response.json(raw);
    },
  });
  assert.equal(requests, 0);
  assert.deepEqual(release, parseRelease(raw));
});

test("public synchronization still works when gh is missing or not logged in", async () => {
  for (const code of ["ENOENT", 1]) {
    const release = await fetchReleaseForSync({
      env: {},
      execFileImpl: async () => { throw Object.assign(new Error("gh unavailable"), { code }); },
      fetchImpl: async (url, options) => {
        assert.equal(url, RELEASE_API_URL);
        assert.equal(options.headers.Authorization, undefined);
        return Response.json(raw);
      },
    });
    assert.deepEqual(release, parseRelease(raw));
  }
});

test("rate limiting reports HTTP status, reset time and authentication instructions", async () => {
  for (const status of [403, 429]) {
    await assert.rejects(fetchReleaseForSync({
      env: {},
      execFileImpl: async () => { throw new Error("gh unavailable"); },
      fetchImpl: async () => Response.json({ message: "API rate limit exceeded" }, {
        status,
        headers: {
          "x-ratelimit-remaining": "0",
          "x-ratelimit-reset": "1792800000",
          "retry-after": "60",
        },
      }),
    }), error => {
      assert.match(error.message, new RegExp(`HTTP ${status}`));
      assert.match(error.message, /rate limit/i);
      assert.ok(error.message.includes(new Date(1792800000 * 1000).toISOString()));
      assert.match(error.message, /60 seconds/);
      assert.match(error.message, /GH_TOKEN\/GITHUB_TOKEN/);
      assert.match(error.message, /gh auth login/);
      return true;
    });
  }
});

test("invalid explicit credentials fail clearly without anonymous fallback or token disclosure", async () => {
  let ghCalls = 0;
  let requests = 0;
  await assert.rejects(fetchReleaseForSync({
    env: { GH_TOKEN: "test-secret-token" },
    execFileImpl: async () => {
      ghCalls++;
      return { stdout: JSON.stringify(raw) };
    },
    fetchImpl: async () => {
      requests++;
      return Response.json({ message: "Bad credentials test-secret-token" }, { status: 401 });
    },
  }), error => {
    assert.match(error.message, /HTTP 401/);
    assert.match(error.message, /GH_TOKEN\/GITHUB_TOKEN/);
    assert.ok(!error.message.includes("test-secret-token"));
    return true;
  });
  assert.equal(ghCalls, 0);
  assert.equal(requests, 1);
});

test("invalid gh release metadata fails validation without falling back", async () => {
  for (const stdout of ["not JSON", JSON.stringify({ ...raw, draft: true })]) {
    let requests = 0;
    await assert.rejects(fetchReleaseForSync({
      env: {},
      execFileImpl: async () => ({ stdout }),
      fetchImpl: async () => {
        requests++;
        return Response.json(raw);
      },
    }), /invalid/);
    assert.equal(requests, 0);
  }
});

test("request failures give safe diagnostics and preserve the saved snapshot", async () => {
  const directory = await fs.mkdtemp(path.join(os.tmpdir(), "passpine-release-sync-"));
  const destination = path.join(directory, "release.json");
  try {
    await fs.writeFile(destination, "previous snapshot\n");
    for (const [fetchImpl, message] of [
      [async () => { throw new TypeError("network failure test-secret-token"); }, /network error/],
      [async () => { throw new DOMException("aborted", "TimeoutError"); }, /timed out/],
      [async () => new Response("invalid JSON test-secret-token"), /not valid JSON/],
      [async () => new Response("unavailable", { status: 503 }), /HTTP 503/],
      [async () => Response.json({ ...raw, draft: true }), /invalid/],
      [async () => Response.json({}, { status: 403, headers: { "x-ratelimit-remaining": "0" } }), /HTTP 403/],
    ]) {
      await assert.rejects(syncRelease({
        destination,
        loadRelease: () => fetchReleaseForSync({
          env: { GH_TOKEN: "test-secret-token" },
          execFileImpl: () => assert.fail("gh must not run with an explicit token"),
          fetchImpl,
        }),
      }), error => {
        assert.match(error.message, message);
        assert.ok(!error.message.includes("test-secret-token"));
        return true;
      });
      assert.equal(await fs.readFile(destination, "utf8"), "previous snapshot\n");
      assert.deepEqual(await fs.readdir(directory), ["release.json"]);
    }
  } finally {
    await fs.rm(directory, { recursive: true, force: true });
  }
});

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
