#!/usr/bin/env node
// Adapted from h2-semver-release for Passpine desktop + browser versions.
// Node.js 22.12+, Git; gh for GitHub releases and initial-release checks.
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
  requireThat(compare(target, current) > 0, 'New version must have greater SemVer precedence; build metadata alone is not an upgrade. Use make release-initial to publish the current version for the first time.');
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
    const detail = phase === 'current-version' ? 'Stopped while checking the current version' : phase === 'before-bump' ? 'Stopped before updating the version' : 'Version edits remain for inspection';
    requireThat(result.status === 0, building
      ? `Build failed: ${command.join(' ')}. Inspect version edits and artifacts before retrying; no push or upload was attempted.`
      : `Check failed: ${command.join(' ')}. ${detail}; no release commit/tag was created.`);
  }
}
function writeVersions(files, target) {
  writeProjectVersions(files, target, process.cwd());
}
export function githubRepo(url, required = true) {
  const match = /^(?:https:\/\/github\.com\/|git@github\.com:|ssh:\/\/git@github\.com\/)([^/]+\/[^/]+?)(?:\.git)?\/?$/.exec(url);
  if (!match && !required) return null;
  requireThat(match, 'GitHub Release requires a standard github.com remote URL; aliases/Enterprise need explicit adaptation.');
  return match[1];
}
function requireUnpublishedRelease(repo, tag) {
  // Successful pagination includes drafts visible to the current credentials;
  // authentication and network failures must never count as an unpublished version.
  const tags = run(['gh', 'api', '--hostname', 'github.com', '--paginate',
    `repos/${repo}/releases`, '--jq', '.[].tag_name']).stdout.split('\n');
  requireThat(!tags.includes(tag), `GitHub Release already exists: ${tag}. Initial release must be unpublished.`);
}
const initialTagMessage = tag => `Release ${tag}\n\nRelease-Type: initial`;
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
    requireThat(release, 'Created release could not be found. Retry with make release VERSION_BUMP=<exact-version> RELEASE_FLAGS="--resume --apply".');
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
    requireThat(missingFrom(remoteAssets()).length === 0, 'Release upload is incomplete; the release remains a draft. Retry with make release VERSION_BUMP=<exact-version> RELEASE_FLAGS="--resume --apply".');
  }
  if (release.isDraft) invoke(['gh', 'release', 'edit', tag, '--repo', githubTarget, '--draft=false', '--verify-tag']);
  const published = view();
  requireThat(published && !published.isDraft, 'Release is still a draft. Retry with make release VERSION_BUMP=<exact-version> RELEASE_FLAGS="--resume --apply".');
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
  requireThat(args.version, 'Use make release-initial, or make release VERSION_BUMP=major|minor|patch|version.');
  requireThat(!(args.resume && args.version === 'initial'), '--resume requires an exact version. Use make release VERSION_BUMP=<version> RELEASE_FLAGS="--resume --apply".');
  return args;
}
function main() {
  const argv = process.argv.slice(2);
  if (argv.includes('--help') || argv.includes('-h')) {
    console.log('Usage: make release-initial [RELEASE_FLAGS="..."]\n       make release VERSION_BUMP=major|minor|patch|version [RELEASE_FLAGS="..."]\nDefaults to preview. release-initial publishes the current version without a version commit; release requires a higher version. RELEASE_FLAGS accepts --apply, --no-push, --github-release, --no-github-release and --resume. Resume with make release VERSION_BUMP=<exact-version> RELEASE_FLAGS="--resume --apply". With ci_release enabled, the pushed tag triggers GitHub Actions to build packages and publish the Release. Local package building and GitHub publication must stay disabled in this mode.');
    return;
  }
  const args = argumentsFor(argv);
  process.chdir(git('rev-parse', '--show-toplevel'));
  const config = fs.existsSync('.release.json') ? readJSON('.release.json') : {};
  requireThat(object(config), '.release.json must contain an object.');
  requireThat(Object.keys(config).every(k => ['version_file', 'branch', 'remote', 'tag_prefix', 'checks', 'check_phase', 'github_release', 'build_artifacts', 'ci_release'].includes(k)), 'Unknown .release.json field.');
  const { branch = 'main', remote = 'origin', tag_prefix: prefix = 'v', checks = [], check_phase: checkPhase = 'before-bump' } = config;
  for (const [name, value] of [['branch', branch], ['remote', remote], ['tag_prefix', prefix]]) {
    requireThat(typeof value === 'string' && (value || name === 'tag_prefix') && !value.startsWith('-'), `Invalid ${name}.`);
  }
  const github = args.github ?? config.github_release ?? false;
  requireThat(typeof github === 'boolean', 'github_release must be boolean.');
  const packaging = config.build_artifacts ?? false;
  requireThat(typeof packaging === 'boolean', 'build_artifacts must be boolean.');
  const ciRelease = config.ci_release ?? false;
  requireThat(typeof ciRelease === 'boolean', 'ci_release must be boolean.');
  requireThat(!ciRelease || (!github && !packaging), 'CI owns release packages and GitHub publication. Keep github_release and build_artifacts disabled while ci_release is enabled.');
  requireThat(!(github && args.noPush), 'GitHub Release requires pushing. Set RELEASE_FLAGS="--no-push --no-github-release" for a local preview or release.');
  requireThat(Array.isArray(checks) && checks.every(c => Array.isArray(c) && c.length && c.every(s => typeof s === 'string' && s)), 'checks must be arrays of command arguments.');
  requireThat(['before-bump', 'after-bump'].includes(checkPhase), 'check_phase must be before-bump or after-bump.');
  clean();
  requireThat(git('symbolic-ref', '--quiet', '--short', 'HEAD') === branch, `Release must run on branch ${branch}.`);
  const head = git('rev-parse', 'HEAD');
  const [current, files] = versionFiles(config);
  for (const name of files.keys()) git('ls-files', '--error-unmatch', '--', name);
  let initial = args.version === 'initial';
  const target = args.resume ? args.version : initial ? current : nextVersion(current, args.version);
  validateVersion(target);
  const bumping = !args.resume && !initial;
  const plan = packaging ? artifactPlan(target) : [];
  const tag = prefix + target;
  git('check-ref-format', 'refs/tags/' + tag);
  const tagExists = run(['git', 'show-ref', '--verify', '--quiet', 'refs/tags/' + tag], false).status === 0;
  if (args.resume) {
    requireThat(current === target && tagExists, 'Resume requires the exact current version and an existing local tag.');
    requireThat(git('rev-parse', tag + '^{commit}') === head, 'Resume tag must point to HEAD.');
    requireThat(git('cat-file', '-t', 'refs/tags/' + tag) === 'tag', 'Resume requires an annotated tag.');
    initial = git('for-each-ref', '--format=%(contents:subject)%0a%0a%(contents:body)', 'refs/tags/' + tag) === initialTagMessage(tag);
    requireThat(initial || git('log', '-1', '--format=%s') === `chore(release): ${tag}`, 'HEAD is not the expected release commit and tag has no initial-release marker. Inspect the original release; do not move or rewrite its tag.');
  } else requireThat(!tagExists, `Tag already exists: ${tag}. Use make release VERSION_BUMP=${target} RELEASE_FLAGS="--resume --apply" after a partial release.`);
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
  const initialRepo = initial && !args.noPush ? githubRepo(url, false) : null;
  if (initialRepo && !args.resume) requireUnpublishedRelease(initialRepo, tag);
  const phase = initial ? 'current-version' : checkPhase;
  const notes = github ? releaseNotes(repo, prefix, target, head, tag) : null;
  console.log(JSON.stringify({ mode: args.apply ? 'execute' : 'preview', release_type: initial ? 'initial' : 'upgrade', current, target, branch, remote, tag, files: bumping ? [...files.keys()] : [], create_version_commit: bumping, checks: args.resume ? [] : checks, check_phase: args.resume ? null : phase, builds: packaging ? artifactBuilds : [], artifacts: plan.map(({ name }) => `dist/releases/${target}/${name}`), reuse_artifacts: args.resume && packaging, push: !args.noPush, ci_release: ciRelease && !args.noPush, github_release: github, github_repo: repo, release_notes: notes, resume: args.resume }, null, 2));
  if (!args.apply) return;
  if (github) run(['gh', 'auth', 'status', '--hostname', 'github.com']);
  let artifacts = args.resume && packaging ? cachedArtifacts(target, head, plan) : null;
  if (!args.resume) {
    if (bumping && checkPhase === 'after-bump') writeVersions(files, target);
    runChecks(checks, files, phase);
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
  if (!args.resume) git('tag', '-a', tag, '-m', initial ? initialTagMessage(tag) : `Release ${tag}`);
  if (!args.noPush) git('push', '--atomic', remote, `HEAD:refs/heads/${branch}`, `refs/tags/${tag}:refs/tags/${tag}`);
  if (github) {
    console.log(publishRelease({ repo, tag, notes, artifacts: artifacts ?? [] }));
  }
  if (ciRelease && !args.noPush) console.log('Tag pushed. GitHub Actions builds and publishes the release: .github/workflows/release.yml');
  console.log(`${ciRelease ? 'Release tag ready' : 'Release complete'}: ${tag}${args.noPush ? ' (local only)' : ''}`);
}
if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  try { main(); }
  catch (error) {
    console.error(`ERROR: ${error.message}\nStopped without automatic rollback or force-push. Inspect git status/log/tags. If the expected release tag already exists at HEAD, retry with make release VERSION_BUMP=<exact-version> RELEASE_FLAGS="--resume --apply".`);
    process.exitCode = 1;
  }
}
