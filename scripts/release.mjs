#!/usr/bin/env node
// Adapted from h2-semver-release for Passpine desktop + browser versions.
// Node.js 22.12+, Git; gh only when selected.
// .mjs works in both CommonJS and ESM projects without extra dependencies.
import fs from 'node:fs';
import { spawnSync } from 'node:child_process';
import { pathToFileURL } from 'node:url';
import { versionFiles as projectVersionFiles, writeVersions as writeProjectVersions, validateVersion } from './version.mjs';
import { artifactPlan, artifactBuilds, buildArtifacts, cachedArtifacts, saveArtifacts } from './release-artifacts.mjs';

function requireThat(condition, message) {
  if (!condition) throw new Error(message);
}
function run(args, check = true, inherit = false, input) {
  const result = spawnSync(args[0], args.slice(1), {
    // Match Python's capture behavior: a Git index or binary diff can exceed 1 MiB.
    encoding: 'utf8', stdio: inherit ? 'inherit' : 'pipe', input, maxBuffer: Infinity,
  });
  if (result.error) throw result.error;
  if (check && result.status !== 0) throw new Error(`Command failed: ${args.join(' ')}\n${(result.stderr ?? '').trim()}`);
  return result;
}
const git = (...args) => run(['git', ...args]).stdout.trim();
const read = name => fs.readFileSync(name, 'utf8');
const readJSON = name => JSON.parse(read(name));
const object = value => value !== null && typeof value === 'object' && !Array.isArray(value);

export function parse(version) {
  const match = typeof version === 'string' && /^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-([0-9A-Za-z.-]+))?(?:\+([0-9A-Za-z.-]+))?$/.exec(version);
  requireThat(match && match[0] === version, `Invalid SemVer: ${version}`);
  const [, major, minor, patch, pre, build] = match;
  for (const group of [pre, build]) {
    if (group !== undefined) requireThat(group.split('.').every(Boolean), `Empty SemVer identifier: ${version}`);
  }
  if (pre) requireThat(pre.split('.').every(x => !/^0[0-9]+$/.test(x)), `Leading zero in prerelease: ${version}`);
  return { core: [major, minor, patch].map(BigInt), pre };
}
function compare(a, b) {
  const left = parse(a), right = parse(b);
  const cmp = (x, y) => x < y ? -1 : x > y ? 1 : 0;
  for (let i = 0; i < 3; i++) {
    const result = cmp(left.core[i], right.core[i]);
    if (result) return result;
  }
  if (left.pre === undefined || right.pre === undefined) return cmp(left.pre === undefined, right.pre === undefined);
  const l = left.pre.split('.'), r = right.pre.split('.');
  for (let i = 0; i < Math.min(l.length, r.length); i++) {
    const ln = /^[0-9]+$/.test(l[i]), rn = /^[0-9]+$/.test(r[i]);
    const result = ln && rn ? cmp(BigInt(l[i]), BigInt(r[i])) : ln !== rn ? (ln ? -1 : 1) : cmp(l[i], r[i]);
    if (result) return result;
  }
  return cmp(l.length, r.length);
}
export function nextVersion(current, bump) {
  const { core, pre } = parse(current);
  const index = ['major', 'minor', 'patch'].indexOf(bump);
  let target = bump;
  if (index !== -1) {
    requireThat(pre === undefined, 'For a prerelease, specify the exact next version (for example 1.2.0).');
    core[index] += 1n;
    core.fill(0n, index + 1);
    target = core.join('.');
  }
  requireThat(compare(target, current) > 0, 'New version must have greater SemVer precedence; build metadata alone is not an upgrade.');
  return target;
}
function versionFiles(config) {
  requireThat(config.version_file === 'package.json', 'Passpine version_file must be package.json.');
  return projectVersionFiles(process.cwd());
}
function clean() {
  requireThat(!git('status', '--porcelain', '--untracked-files=all'), 'Working tree must be clean, including untracked files. Review and commit your changes first.');
}
function checkState() {
  return {
    head: git('rev-parse', 'HEAD'),
    branch: run(['git', 'symbolic-ref', '--quiet', 'HEAD'], false).stdout.trim(),
    refs: git('for-each-ref', '--format=%(refname) %(objectname)', 'refs/heads', 'refs/tags'),
    index: run(['git', 'ls-files', '--stage', '-z']).stdout,
    status: run(['git', 'status', '--porcelain=v1', '-z', '--untracked-files=all']).stdout,
    diff: run(['git', 'diff', '--binary', '--no-ext-diff', '--no-textconv', '--no-color']).stdout,
  };
}
function runChecks(checks, files, phase, building = false) {
  if (!checks.length) return;
  const before = checkState();
  const versions = new Map([...files.keys()].map(name => [name, read(name)]));
  for (const command of checks) {
    const result = run(command, false, true);
    const after = checkState();
    requireThat(after.head === before.head, 'A check changed HEAD. Inspect repository before continuing.');
    requireThat(after.branch === before.branch, 'A check changed branches.');
    requireThat(after.refs === before.refs, 'A check changed local branches or tags. Inspect repository before continuing.');
    requireThat(after.index === before.index, 'A check changed the index. Inspect repository before continuing.');
    requireThat([...versions].every(([name, content]) => fs.existsSync(name) && fs.lstatSync(name).isFile() && read(name) === content), 'A check changed version files. Inspect and resolve manually before retrying.');
    requireThat(after.status === before.status && after.diff === before.diff, 'Checks changed the working tree. Inspect and resolve manually before retrying.');
    const detail = phase === 'before-bump' ? 'Stopped before updating the version' : 'Version edits remain for inspection';
    requireThat(result.status === 0, building
      ? `Build failed: ${command.join(' ')}. Inspect version edits and artifacts before retrying; no push or upload was attempted.`
      : `Check failed: ${command.join(' ')}. ${detail}; no release commit/tag was created.`);
  }
}
function writeVersions(files, target) {
  writeProjectVersions(files, target, process.cwd());
}
export function githubRepo(url) {
  const match = /^(?:https:\/\/github\.com\/|git@github\.com:|ssh:\/\/git@github\.com\/)([^/]+\/[^/]+?)(?:\.git)?\/?$/.exec(url);
  requireThat(match, 'GitHub Release requires a standard github.com remote URL; aliases/Enterprise need explicit adaptation.');
  return match[1];
}
export function releaseNotes(repo, prefix, target, sourceHead, tag) {
  requireThat(git('rev-parse', '--is-shallow-repository') === 'false', 'Release notes require complete Git history and tags; fetch them before generating notes.');
  const previous = git('tag', '--merged', sourceHead, '--list').split('\n').filter(Boolean)
    .filter(name => name.startsWith(prefix))
    .filter(name => {
      try { return compare(name.slice(prefix.length), target) < 0; }
      catch { return false; }
    })
    .sort((a, b) => compare(b.slice(prefix.length), a.slice(prefix.length)))[0];
  const range = previous ? `refs/tags/${previous}..${sourceHead}` : sourceHead;
  const commits = git('log', '--no-merges', '--format=%h%x1f%s', range).split('\n').filter(Boolean)
    .map(line => line.split('\x1f'))
    .filter(([, subject]) => !subject.startsWith('chore(release): '));
  const path = previous
    ? `compare/${encodeURIComponent(previous)}...${encodeURIComponent(tag)}`
    : `commits/${encodeURIComponent(tag)}`;
  const entries = commits.length
    ? commits.map(([hash, subject]) => `- ${subject.replace(/[\\`*_{}\[\]()#+.!|>~-]/g, '\\$&')} (\`${hash}\`)`).join('\n')
    : '- No changes since the previous release.';
  return `**Full Changelog**: https://github.com/${repo}/${path}\n\n### Commits\n\n${entries}\n`;
}

// Keep incomplete uploads in a draft, and never replace an existing asset.
export function publishRelease({ repo, tag, notes, artifacts }, invoke = run) {
  const githubTarget = `github.com/${repo}`;
  const view = () => {
    const result = invoke(['gh', 'release', 'view', tag, '--repo', githubTarget, '--json', 'databaseId,url,isDraft,tagName'], false);
    if (result.status === 0) {
      const release = JSON.parse(result.stdout);
      requireThat(release.tagName === tag && Number.isSafeInteger(release.databaseId) && typeof release.isDraft === 'boolean' && typeof release.url === 'string', 'Unexpected GitHub Release metadata.');
      return release;
    }
    requireThat(/release not found/i.test(result.stderr ?? ''), `Cannot inspect GitHub Release: ${result.stderr}`);
    return null;
  };
  let release = view();
  if (!release) {
    invoke(['gh', 'release', 'create', tag, '--repo', githubTarget, '--verify-tag', '--draft', '--notes-file', '-', '--title', tag], true, false, notes);
    release = view();
    requireThat(release, 'Created release could not be found. Retry with --resume.');
  }
  const remoteAssets = () => {
    const result = invoke(['gh', 'api', `repos/${repo}/releases/${release.databaseId}/assets?per_page=100`, '--hostname', 'github.com', '--paginate', '--slurp']);
    return JSON.parse(result.stdout).flat();
  };
  const missingFrom = remote => artifacts.filter(artifact => {
    const matches = remote.filter(item => item.name === artifact.name);
    requireThat(matches.length <= 1, `Duplicate release asset: ${artifact.name}`);
    if (!matches.length) return true;
    const [existing] = matches;
    requireThat(existing.state === 'uploaded' && existing.size === artifact.size && existing.digest === `sha256:${artifact.sha256}`,
      `Existing release asset differs from the local package: ${artifact.name}. Refusing to overwrite it.`);
    return false;
  });
  const missing = missingFrom(remoteAssets());
  requireThat(release.isDraft || missing.length === 0, 'Published release is missing expected assets. Inspect it before making changes.');
  if (missing.length) {
    invoke(['gh', 'release', 'upload', tag, ...missing.map(artifact => artifact.path), '--repo', githubTarget]);
    requireThat(missingFrom(remoteAssets()).length === 0, 'Release upload is incomplete; the release remains a draft. Retry with --resume.');
  }
  if (release.isDraft) invoke(['gh', 'release', 'edit', tag, '--repo', githubTarget, '--draft=false', '--verify-tag']);
  const published = view();
  requireThat(published && !published.isDraft, 'Release is still a draft. Retry with --resume.');
  return published.url;
}

function argumentsFor(argv) {
  const args = { apply: false, noPush: false, resume: false };
  for (const arg of argv) {
    if (arg === '--apply') args.apply = true;
    else if (arg === '--no-push') args.noPush = true;
    else if (arg === '--resume') args.resume = true;
    else if (['--github-release', '--no-github-release'].includes(arg)) {
      const value = arg === '--github-release';
      requireThat(args.github === undefined || args.github === value, 'GitHub Release flags are mutually exclusive.');
      args.github = value;
    } else {
      requireThat(!arg.startsWith('-') && args.version === undefined, `Unexpected argument: ${arg}`);
      args.version = arg;
    }
  }
  requireThat(args.version, 'Expected major, minor, patch, or exact SemVer. Use --help for usage.');
  return args;
}
function main() {
  const argv = process.argv.slice(2);
  if (argv.includes('--help') || argv.includes('-h')) {
    console.log('Usage: node scripts/release.mjs <major|minor|patch|version> [--apply] [--no-push] [--github-release|--no-github-release] [--resume]\nDefaults to preview. An untagged current version can be released without a bump. --resume requires the exact existing release version. Configured artifacts are built before tagging and uploaded before publishing the GitHub Release.');
    return;
  }
  const args = argumentsFor(argv);
  process.chdir(git('rev-parse', '--show-toplevel'));
  const config = fs.existsSync('.release.json') ? readJSON('.release.json') : {};
  requireThat(object(config), '.release.json must contain an object.');
  requireThat(Object.keys(config).every(k => ['version_file', 'branch', 'remote', 'tag_prefix', 'checks', 'check_phase', 'github_release', 'build_artifacts'].includes(k)), 'Unknown .release.json field.');
  const { branch = 'main', remote = 'origin', tag_prefix: prefix = 'v', checks = [], check_phase: checkPhase = 'before-bump' } = config;
  for (const [name, value] of [['branch', branch], ['remote', remote], ['tag_prefix', prefix]]) {
    requireThat(typeof value === 'string' && (value || name === 'tag_prefix') && !value.startsWith('-'), `Invalid ${name}.`);
  }
  const github = args.github ?? config.github_release ?? false;
  requireThat(typeof github === 'boolean', 'github_release must be boolean.');
  const packaging = config.build_artifacts ?? false;
  requireThat(typeof packaging === 'boolean', 'build_artifacts must be boolean.');
  requireThat(!(github && args.noPush), 'GitHub Release requires pushing. Use --no-github-release with --no-push.');
  requireThat(Array.isArray(checks) && checks.every(c => Array.isArray(c) && c.length && c.every(s => typeof s === 'string' && s)), 'checks must be arrays of command arguments.');
  requireThat(['before-bump', 'after-bump'].includes(checkPhase), 'check_phase must be before-bump or after-bump.');
  clean();
  requireThat(git('symbolic-ref', '--quiet', '--short', 'HEAD') === branch, `Release must run on branch ${branch}.`);
  const head = git('rev-parse', 'HEAD');
  const [current, files] = versionFiles(config);
  for (const name of files.keys()) git('ls-files', '--error-unmatch', '--', name);
  const target = args.resume || args.version === current ? args.version : nextVersion(current, args.version);
  validateVersion(target);
  const bumping = !args.resume && target !== current;
  const plan = packaging ? artifactPlan(target) : [];
  const tag = prefix + target;
  git('check-ref-format', 'refs/tags/' + tag);
  const tagExists = run(['git', 'show-ref', '--verify', '--quiet', 'refs/tags/' + tag], false).status === 0;
  if (args.resume) {
    requireThat(current === target && tagExists, 'Resume requires the exact current version and an existing local tag.');
    requireThat(git('rev-parse', tag + '^{commit}') === head, 'Resume tag must point to HEAD.');
    requireThat(git('cat-file', '-t', 'refs/tags/' + tag) === 'tag', 'Resume requires an annotated tag.');
  } else requireThat(!tagExists, `Tag already exists: ${tag}. Use --resume with the exact version after a partial release.`);
  let url;
  if (!args.noPush) {
    const urls = git('remote', 'get-url', '--push', '--all', remote).split('\n');
    requireThat(urls.length === 1 && urls[0], 'Exactly one push URL is required.');
    [url] = urls;
    const refs = new Map(git('ls-remote', url, 'refs/heads/' + branch, 'refs/tags/' + tag).split('\n').filter(Boolean).map(line => {
      const [sha, ref] = line.split(/\s+/);
      return [ref, sha];
    }));
    const remoteHead = refs.get('refs/heads/' + branch);
    requireThat(remoteHead, `Remote branch ${branch} is missing; establish the branch before releasing.`);
    requireThat(run(['git', 'merge-base', '--is-ancestor', remoteHead, head], false).status === 0, 'Remote branch is ahead/diverged or unavailable locally. Fetch and reconcile before releasing.');
    const remoteTag = refs.get('refs/tags/' + tag);
    if (remoteTag) requireThat(args.resume && remoteTag === git('rev-parse', 'refs/tags/' + tag), 'Remote tag already exists with a different release; never overwrite it.');
  }
  const repo = github ? githubRepo(url) : null;
  const notes = github ? releaseNotes(repo, prefix, target, head, tag) : null;
  console.log(JSON.stringify({ mode: args.apply ? 'execute' : 'preview', current, target, branch, remote, tag, files: bumping ? [...files.keys()] : [], checks: args.resume ? [] : checks, check_phase: args.resume ? null : checkPhase, builds: packaging ? artifactBuilds : [], artifacts: plan.map(({ name }) => `dist/releases/${target}/${name}`), reuse_artifacts: args.resume && packaging, push: !args.noPush, github_release: github, github_repo: repo, release_notes: notes, resume: args.resume }, null, 2));
  if (!args.apply) return;
  if (github) run(['gh', 'auth', 'status', '--hostname', 'github.com']);
  let artifacts = args.resume && packaging ? cachedArtifacts(target, head, plan) : null;
  if (!args.resume) {
    if (bumping && checkPhase === 'after-bump') writeVersions(files, target);
    runChecks(checks, files, checkPhase);
    if (bumping && checkPhase === 'before-bump') writeVersions(files, target);
  }
  if (packaging && !artifacts) artifacts = buildArtifacts(target, plan, command => runChecks([command], files, 'after-bump', true));
  if (!args.resume) {
    if (bumping) {
      const changed = git('diff', '--name-only').split('\n').filter(Boolean);
      requireThat(changed.length === files.size && changed.every(name => files.has(name)) && !git('ls-files', '--others', '--exclude-standard'), 'Unexpected changes after updating the version. Inspect and resolve manually before retrying.');
      requireThat(versionFiles(config)[0] === target, 'Release version differs from the target.');
      git('add', '--', ...files.keys());
      git('commit', '-m', `chore(release): ${tag}`);
      clean();
      requireThat(git('symbolic-ref', '--short', 'HEAD') === branch, 'A hook changed branches.');
      requireThat(versionFiles(config)[0] === target, 'A commit hook changed the version.');
    } else clean();
  }
  if (packaging) saveArtifacts(target, git('rev-parse', 'HEAD'), artifacts);
  if (!args.resume) git('tag', '-a', tag, '-m', `Release ${tag}`);
  if (!args.noPush) git('push', '--atomic', remote, `HEAD:refs/heads/${branch}`, `refs/tags/${tag}:refs/tags/${tag}`);
  if (github) {
    console.log(publishRelease({ repo, tag, notes, artifacts: artifacts ?? [] }));
  }
  console.log(`Release complete: ${tag}${args.noPush ? ' (local only)' : ''}`);
}
if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  try { main(); }
  catch (error) {
    console.error(`ERROR: ${error.message}\nStopped without automatic rollback or force-push. Inspect git status/log/tags. If commit and tag already exist at HEAD, retry with the exact version --resume --apply.`);
    process.exitCode = 1;
  }
}
