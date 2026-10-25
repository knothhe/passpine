#!/usr/bin/env node
// Stable macOS code-signing identity for release builds. A self-signed
// certificate keeps the designated requirement identical across releases, so
// login items and other identity-bound grants survive updates. It does not
// replace Developer ID notarization for Gatekeeper.
import crypto from 'node:crypto';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { execFileSync, spawnSync } from 'node:child_process';
import { fileURLToPath, pathToFileURL } from 'node:url';
import readline from 'node:readline/promises';

const projectRoot = fileURLToPath(new URL('../', import.meta.url));
export const defaultConfigPath = path.join(projectRoot, 'config/macos-signing.json');
export const defaultIdentityDir = path.join(os.homedir(), '.config/passpine-release/macos-signing');
const commonName = 'Passpine Release Signing (self-signed)';
const codeSigningOID = '1.3.6.1.5.5.7.3.3';
const certificateEnv = 'PASSPINE_MACOS_SIGNING_CERTIFICATE';
const passwordEnv = 'PASSPINE_MACOS_SIGNING_CERTIFICATE_PASSWORD';

const requireThat = (condition, message) => { if (!condition) throw new Error(message); };
const sha256 = bytes => crypto.createHash('sha256').update(bytes).digest('hex');

export function readSigningConfig(filename = defaultConfigPath) {
  requireThat(fs.existsSync(filename), 'macOS release signing is not configured: run `node scripts/macos-signing.mjs` once to create or choose the identity and upload the Actions secrets.');
  const config = JSON.parse(fs.readFileSync(filename, 'utf8'));
  requireThat(config && typeof config === 'object' && Object.keys(config).sort().join(',') === 'certificateSHA256,schema', 'macOS signing configuration must contain only schema and certificateSHA256.');
  requireThat(config.schema === 1 && /^[a-f0-9]{64}$/.test(config.certificateSHA256), 'Invalid macOS signing configuration.');
  return config;
}

export function certificateFingerprint(der) {
  const certificate = new crypto.X509Certificate(der);
  requireThat(certificate.keyUsage?.includes(codeSigningOID), 'Signing certificate must allow code signing.');
  return sha256(certificate.raw);
}

// `security find-identity` lists untrusted self-signed identities too; the
// trust state is irrelevant because the pinned certificate is checked after signing.
export function parseIdentityList(output) {
  const identities = new Map();
  for (const match of output.matchAll(/^\s*\d+\)\s+([0-9A-F]{40})\s+"(.*)"(?:\s+\((.+)\))?\s*$/gm)) {
    if (!identities.has(match[1])) identities.set(match[1], { hash: match[1], name: match[2], status: match[3] ?? '' });
  }
  return [...identities.values()];
}

export function parseIdentities(output) {
  return parseIdentityList(output).map(identity => identity.hash);
}

// Picks one certificate and its private key from a PEM dump of a keychain
// export, which may contain every identity in that keychain.
export function selectIdentity(pem, sha1) {
  const blocks = [...pem.matchAll(/-----BEGIN ([A-Z ]+)-----[\s\S]+?-----END \1-----/g)];
  const spki = key => key.export({ type: 'spki', format: 'der' }).toString('hex');
  const certificate = blocks.filter(block => block[1] === 'CERTIFICATE').map(block => new crypto.X509Certificate(block[0]))
    .find(item => item.fingerprint.replaceAll(':', '') === sha1.toUpperCase());
  requireThat(certificate, 'The selected certificate is missing from the keychain export.');
  const key = blocks.filter(block => block[1].endsWith('PRIVATE KEY')).map(block => crypto.createPrivateKey(block[0]))
    .find(item => spki(crypto.createPublicKey(item)) === spki(certificate.publicKey));
  requireThat(key, 'The private key of the selected certificate is missing from the keychain export.');
  return { certificate: certificate.toString(), key: key.export({ type: 'pkcs8', format: 'pem' }) };
}

export function assertCertificateRequirement(requirement) {
  requireThat(/designated => .*certificate leaf = H"[0-9a-f]{40}"/i.test(requirement) && !/cdhash/i.test(requirement), 'Signed bundle must use a certificate-based designated requirement.');
}

export function generate({ output = defaultIdentityDir, config = defaultConfigPath, force = false, now = new Date() } = {}) {
  const files = ['certificate.pem', 'identity.p12', 'identity-password'].map(name => path.join(output, name));
  const existing = files.filter(file => fs.existsSync(file));
  requireThat(force || (!fs.existsSync(config) && !existing.length), `A macOS signing identity already exists (${fs.existsSync(config) ? 'config/macos-signing.json is pinned' : output}). Run \`node scripts/macos-signing.mjs\` to reuse or replace it; replacing changes the app identity for every installed client.`);
  fs.mkdirSync(output, { recursive: true, mode: 0o700 });
  fs.chmodSync(output, 0o700);
  let replaced;
  if (existing.length) {
    // Keep the previous identity so a mistaken replacement can be undone.
    replaced = path.join(output, `replaced-${now.toISOString().replace(/[:.]/g, '-')}`);
    fs.mkdirSync(replaced, { mode: 0o700 });
    for (const file of existing) fs.renameSync(file, path.join(replaced, path.basename(file)));
  }
  const work = fs.mkdtempSync(path.join(os.tmpdir(), 'passpine-macos-signing-'));
  try {
    const request = path.join(work, 'request.cnf'), key = path.join(work, 'key.pem');
    fs.writeFileSync(request, `[req]\ndistinguished_name=dn\nx509_extensions=ext\nprompt=no\n[dn]\nCN=${commonName}\n[ext]\nbasicConstraints=critical,CA:FALSE\nkeyUsage=critical,digitalSignature\nextendedKeyUsage=critical,codeSigning\nsubjectKeyIdentifier=hash\n`);
    fs.writeFileSync(files[2], crypto.randomBytes(32).toString('base64url'), { mode: 0o600 });
    const openssl = args => execFileSync('openssl', args, { stdio: ['ignore', 'pipe', 'pipe'] });
    openssl(['req', '-x509', '-newkey', 'rsa:3072', '-nodes', '-sha256', '-days', '10950', '-config', request, '-keyout', key, '-out', files[0]]);
    // macOS `security import` needs the legacy PKCS#12 algorithms.
    openssl(['pkcs12', '-export', '-inkey', key, '-in', files[0], '-out', files[1], '-name', commonName, '-passout', `file:${files[2]}`,
      '-keypbe', 'PBE-SHA1-3DES', '-certpbe', 'PBE-SHA1-3DES', '-macalg', 'sha1']);
    for (const file of files) fs.chmodSync(file, 0o600);
  } finally {
    fs.rmSync(work, { recursive: true, force: true });
  }
  const fingerprint = certificateFingerprint(fs.readFileSync(files[0]));
  fs.writeFileSync(config, `${JSON.stringify({ schema: 1, certificateSHA256: fingerprint }, null, 2)}\n`);
  return { fingerprint, certificate: files[0], identity: files[1], password: files[2], replaced };
}

function codesignDetails(target, ...args) {
  const result = spawnSync('/usr/bin/codesign', ['-d', ...args, target], { encoding: 'utf8' });
  requireThat(result.status === 0, `codesign could not read ${target}: ${result.stderr.trim()}`);
  return `${result.stdout}${result.stderr}`;
}

export function verify(bundle, config = readSigningConfig()) {
  const work = fs.mkdtempSync(path.join(os.tmpdir(), 'passpine-signature-'));
  try {
    const executables = fs.readdirSync(path.join(bundle, 'Contents/MacOS')).map(name => path.join(bundle, 'Contents/MacOS', name));
    for (const [index, target] of [bundle, ...executables].entries()) {
      const prefix = path.join(work, `${index}-`);
      codesignDetails(target, `--extract-certificates=${prefix}`);
      requireThat(certificateFingerprint(fs.readFileSync(`${prefix}0`)) === config.certificateSHA256, `${target} is not signed with the pinned macOS release certificate.`);
      assertCertificateRequirement(codesignDetails(target, '-r-'));
    }
  } finally {
    fs.rmSync(work, { recursive: true, force: true });
  }
}

// Imports the CI identity into a throwaway keychain only for the release
// build, then restores the keychain search list.
export function buildRelease(env = process.env, root = projectRoot) {
  requireThat(process.platform === 'darwin' && env.GITHUB_ACTIONS === 'true' && env.RUNNER_TEMP, 'Release signing runs only on an ephemeral macOS GitHub Actions runner.');
  requireThat(env[certificateEnv] && env[passwordEnv], `Missing ${certificateEnv} or ${passwordEnv} Actions secret.`);
  readSigningConfig(path.join(root, 'config/macos-signing.json'));
  const work = fs.mkdtempSync(path.join(fs.realpathSync(env.RUNNER_TEMP), 'passpine-signing-'));
  const keychain = path.join(work, 'release.keychain-db'), identity = path.join(work, 'identity.p12');
  const password = crypto.randomBytes(24).toString('base64url');
  const security = (...args) => execFileSync('/usr/bin/security', args, { encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'] });
  const searchList = security('list-keychains', '-d', 'user').split('\n').map(line => line.trim().replace(/^"|"$/g, '')).filter(Boolean);
  let created = false;
  try {
    fs.writeFileSync(identity, Buffer.from(env[certificateEnv], 'base64'), { mode: 0o600 });
    security('create-keychain', '-p', password, keychain); created = true;
    security('set-keychain-settings', '-lut', '3600', keychain);
    security('unlock-keychain', '-p', password, keychain);
    security('import', identity, '-k', keychain, '-f', 'pkcs12', '-P', env[passwordEnv], '-T', '/usr/bin/codesign');
    security('set-key-partition-list', '-S', 'apple-tool:,apple:,codesign:', '-s', '-k', password, keychain);
    security('list-keychains', '-d', 'user', '-s', keychain, ...searchList);
    const identities = parseIdentities(security('find-identity', '-p', 'codesigning', keychain));
    requireThat(identities.length === 1, `Expected one macOS signing identity, found ${identities.length}.`);
    const childEnv = { ...env, PASSPINE_MACOS_SIGNING_IDENTITY: identities[0], PASSPINE_MACOS_SIGNING_KEYCHAIN: keychain };
    delete childEnv[certificateEnv]; delete childEnv[passwordEnv];
    const build = spawnSync('bash', [path.join(root, 'scripts/build-client.sh')], { cwd: root, env: childEnv, stdio: 'inherit' });
    requireThat(build.status === 0, `Signed macOS build failed (${build.status ?? build.signal}).`);
  } finally {
    if (created) {
      try { security('list-keychains', '-d', 'user', '-s', ...searchList); } finally { security('delete-keychain', keychain); }
    }
    fs.rmSync(work, { recursive: true, force: true });
  }
}

function keychainIdentities() {
  const security = (...args) => execFileSync('/usr/bin/security', args, { encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'] });
  const keychains = security('list-keychains', '-d', 'user').split('\n').map(line => line.trim().replace(/^"|"$/g, '')).filter(Boolean);
  return keychains.flatMap(keychain => parseIdentityList(security('find-identity', '-p', 'codesigning', keychain)).map(identity => ({ ...identity, keychain })));
}

function uploadSecrets(repository, identity, password) {
  execFileSync('gh', ['secret', 'set', certificateEnv, '--repo', repository], { input: identity.toString('base64'), stdio: ['pipe', 'inherit', 'inherit'] });
  execFileSync('gh', ['secret', 'set', passwordEnv, '--repo', repository], { input: password, stdio: ['pipe', 'inherit', 'inherit'] });
}

const writePin = (config, fingerprint) => fs.writeFileSync(config, `${JSON.stringify({ schema: 1, certificateSHA256: fingerprint }, null, 2)}\n`);

// Reads the identity kept by `generate`, if any, and whether it is the pinned one.
export function localIdentity(output = defaultIdentityDir, pinned) {
  const files = { certificate: path.join(output, 'certificate.pem'), identity: path.join(output, 'identity.p12'), password: path.join(output, 'identity-password') };
  if (!Object.values(files).every(file => fs.existsSync(file))) return undefined;
  const fingerprint = certificateFingerprint(fs.readFileSync(files.certificate));
  return { ...files, fingerprint, pinned: fingerprint === pinned };
}

// The setup menu offers only actions that make sense for the current state.
export function setupChoices({ pinned, local, platform = process.platform }) {
  const choices = [];
  if (local && (!pinned || local.pinned)) choices.push({ action: 'reupload', label: `Upload the local identity in ${path.dirname(local.identity)}${pinned ? '' : ' and pin it'}` });
  if (platform === 'darwin') choices.push({ action: 'keychain', label: 'Choose an existing code-signing identity from the macOS keychain' });
  choices.push({ action: 'generate', label: pinned ? 'Generate a new self-signed identity and replace the pinned one' : 'Generate a new self-signed identity' });
  return choices;
}

async function confirmReplacement(prompt, pinned, fingerprint) {
  if (!pinned || pinned === fingerprint) return;
  console.log(`\nThis replaces the pinned certificate ${pinned}.\nEvery installed client will see a new app identity once; login items may need approval again.`);
  requireThat((await prompt.question('Type replace to continue: ')).trim() === 'replace', 'Cancelled; nothing was changed.');
}

async function chooseKeychainIdentity(prompt) {
  const identities = keychainIdentities();
  requireThat(identities.length, 'No code-signing identity was found in the keychain search list.');
  console.log('');
  identities.forEach((identity, index) => console.log(`${index + 1}) ${identity.name}${identity.status ? ` (${identity.status})` : ''}\n   ${identity.hash}  ${identity.keychain}`));
  const choice = Number(await prompt.question('Select a signing certificate: '));
  requireThat(Number.isInteger(choice) && identities[choice - 1], 'No signing certificate selected.');
  return identities[choice - 1];
}

// Exports one keychain identity as a fresh PKCS#12 with a random password.
// `security export` cannot pick one identity, so the full export stays in a
// private temporary directory and only the selected pair is repacked.
function exportKeychainIdentity(identity, work) {
  const exportPassword = crypto.randomBytes(24).toString('base64url'), password = crypto.randomBytes(32).toString('base64url');
  const files = Object.fromEntries(['all.p12', 'export-password', 'certificate.pem', 'key.pem', 'identity.p12', 'password'].map(name => [name, path.join(work, name)]));
  fs.writeFileSync(files['export-password'], exportPassword, { mode: 0o600 });
  fs.writeFileSync(files.password, password, { mode: 0o600 });
  console.log('macOS may ask for permission to export the keychain private keys.');
  execFileSync('/usr/bin/security', ['export', '-k', identity.keychain, '-t', 'identities', '-f', 'pkcs12', '-P', exportPassword, '-o', files['all.p12']], { stdio: ['ignore', 'ignore', 'inherit'] });
  const openssl = args => execFileSync('/usr/bin/openssl', args, { encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'] });
  const selected = selectIdentity(openssl(['pkcs12', '-in', files['all.p12'], '-nodes', '-passin', `file:${files['export-password']}`]), identity.hash);
  fs.writeFileSync(files['certificate.pem'], selected.certificate, { mode: 0o600 });
  fs.writeFileSync(files['key.pem'], selected.key, { mode: 0o600 });
  openssl(['pkcs12', '-export', '-inkey', files['key.pem'], '-in', files['certificate.pem'], '-out', files['identity.p12'], '-name', identity.name,
    '-passout', `file:${files.password}`, '-keypbe', 'PBE-SHA1-3DES', '-certpbe', 'PBE-SHA1-3DES']);
  const certificate = new crypto.X509Certificate(selected.certificate);
  if (Date.parse(certificate.validTo) < Date.now()) console.log(`Warning: this certificate expired on ${certificate.validTo}.`);
  console.log(`\nCertificate: ${certificate.subject.replaceAll('\n', ', ')}`);
  return { fingerprint: certificateFingerprint(certificate.raw), identity: fs.readFileSync(files['identity.p12']), password };
}

// Interactive maintainer setup: pick or create the release identity, upload
// both Actions secrets through gh and keep config/macos-signing.json in sync.
export async function setup({ config = defaultConfigPath, output = defaultIdentityDir } = {}) {
  requireThat(process.stdin.isTTY && process.stdout.isTTY, 'Signing setup is interactive; run it in a terminal.');
  execFileSync('gh', ['auth', 'status'], { stdio: 'ignore' });
  const terminal = readline.createInterface({ input: process.stdin, output: process.stdout });
  // Ctrl-D or Ctrl-C at a prompt cancels instead of surfacing an AbortError.
  const prompt = { question: query => terminal.question(query).catch(error => { throw error?.code === 'ABORT_ERR' ? new Error('Cancelled.') : error; }) };
  const work = fs.mkdtempSync(path.join(os.tmpdir(), 'passpine-signing-setup-'));
  try {
    const pinned = fs.existsSync(config) ? readSigningConfig(config).certificateSHA256 : undefined;
    const local = localIdentity(output, pinned);
    console.log('Passpine macOS release signing\n');
    console.log(`Pinned certificate: ${pinned ?? 'none'}`);
    console.log(`Local identity:     ${local ? `${local.fingerprint}${local.pinned ? ' (pinned)' : ''}` : 'none'} in ${output}`);
    const repository = (await prompt.question('\nGitHub repository [knothhe/passpine]: ')).trim() || 'knothhe/passpine';
    requireThat(/^[\w.-]+\/[\w.-]+$/.test(repository), 'Invalid repository; expected OWNER/NAME.');
    const choices = setupChoices({ pinned, local });
    console.log('');
    choices.forEach((choice, index) => console.log(`${index + 1}) ${choice.label}`));
    console.log('q) Quit');
    const answer = (await prompt.question('Choose an action: ')).trim();
    const choice = choices[Number(answer) - 1];
    requireThat(choice, 'Cancelled; nothing was changed.');
    let result;
    if (choice.action === 'reupload') {
      result = { fingerprint: local.fingerprint, identity: fs.readFileSync(local.identity), password: fs.readFileSync(local.password, 'utf8') };
    } else if (choice.action === 'keychain') {
      result = exportKeychainIdentity(await chooseKeychainIdentity(prompt), work);
      await confirmReplacement(prompt, pinned, result.fingerprint);
    } else {
      await confirmReplacement(prompt, pinned, undefined);
      const generated = generate({ output, config, force: true });
      if (generated.replaced) console.log(`Moved the previous local identity to ${generated.replaced}.`);
      console.log(`Generated ${generated.fingerprint} in ${output}; keep this directory backed up offline.`);
      result = { fingerprint: generated.fingerprint, identity: fs.readFileSync(generated.identity), password: fs.readFileSync(generated.password, 'utf8') };
    }
    console.log(`\nSHA-256:    ${result.fingerprint}\nRepository: ${repository}`);
    if ((await prompt.question(`Upload ${certificateEnv} and ${passwordEnv} now? [Y/n] `)).trim().toLowerCase().startsWith('n')) {
      if (choice.action === 'generate') console.log('Not uploaded. Run this setup again and choose the local identity to upload it.');
      else throw new Error('Cancelled; nothing was uploaded.');
    } else {
      uploadSecrets(repository, result.identity, result.password);
      console.log('Uploaded both Actions secrets.');
    }
    if (pinned !== result.fingerprint) {
      writePin(config, result.fingerprint);
      console.log(`Pinned ${result.fingerprint} in config/macos-signing.json; commit it before the next release.`);
    }
  } finally {
    terminal.close();
    fs.rmSync(work, { recursive: true, force: true });
  }
}

if (process.argv[1] && pathToFileURL(path.resolve(process.argv[1])).href === import.meta.url) {
  try {
    const [action, ...args] = process.argv.slice(2);
    if (action === undefined) {
      await setup();
    } else if (action === 'verify' && args.length === 1) {
      verify(path.resolve(args[0]));
      console.log(`Verified the pinned release signature: ${args[0]}`);
    } else if (action === 'build' && args.length === 0) {
      buildRelease();
    } else {
      throw new Error('Usage: macos-signing.mjs (interactive setup) | verify APP_BUNDLE | build');
    }
  } catch (error) {
    console.error(`macOS signing failed: ${error.message}`);
    process.exitCode = 1;
  }
}
