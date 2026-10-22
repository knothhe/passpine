#!/usr/bin/env node
import fs from 'node:fs';
import { execFileSync } from 'node:child_process';
import { pathToFileURL } from 'node:url';
import { releaseNotes } from './release.mjs';
import { validateVersion } from './version.mjs';

export function tagVersion(tag, root = process.cwd()) {
  const { tag_prefix: prefix = 'v' } = JSON.parse(fs.readFileSync(`${root}/.release.json`, 'utf8'));
  if (typeof tag !== 'string' || !tag.startsWith(prefix)) throw new Error('Expected a release tag with the configured prefix.');
  return validateVersion(tag.slice(prefix.length));
}

export function notesForTag(repo, tag, root = process.cwd()) {
  if (!/^[\w.-]+\/[\w.-]+$/.test(repo)) throw new Error('Expected GitHub owner/repository.');
  const version = tagVersion(tag, root);
  const head = execFileSync('git', ['rev-parse', '--verify', `refs/tags/${tag}^{commit}`], { cwd: root, encoding: 'utf8' }).trim();
  const { tag_prefix: prefix = 'v' } = JSON.parse(fs.readFileSync(`${root}/.release.json`, 'utf8'));
  // releaseNotes reads Git history in the current working directory.
  const previous = process.cwd();
  try { process.chdir(root); return releaseNotes(repo, prefix, version, head, tag); }
  finally { process.chdir(previous); }
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  try {
    const [repo, tag, ...rest] = process.argv.slice(2);
    if (rest.length) throw new Error('Usage: node scripts/release-notes.mjs owner/repository tag');
    process.stdout.write(notesForTag(repo, tag));
  } catch (error) { console.error(`ERROR: ${error.message}`); process.exitCode = 1; }
}
