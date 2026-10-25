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
  requireThat(fs.existsSync(filename), 'macOS release signing is not configured: run `node scripts/macos-signing.mjs generate` once and store the identity in the Actions secrets.');
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

export function generate({ output = defaultIdentityDir, config = defaultConfigPath } = {}) {
  const files = ['certificate.pem', 'identity.p12', 'identity-password'].map(name => path.join(output, name));
  requireThat(!fs.existsSync(config) && files.every(file => !fs.existsSync(file)), 'A macOS signing identity already exists; replacing it changes the app identity for every installed client.');
  fs.mkdirSync(output, { recursive: true, mode: 0o700 });
  fs.chmodSync(output, 0o700);
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
  return { fingerprint, certificate: files[0], identity: files[1], password: files[2] };
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

// Uploads an existing keychain identity as the release signing secrets and
// pins its certificate. Runs interactively on the maintainer's Mac.
export async function uploadKeychainIdentity({ repository = 'knothhe/passpine', config = defaultConfigPath } = {}) {
  requireThat(process.platform === 'darwin', 'Uploading a keychain identity requires macOS.');
  execFileSync('gh', ['auth', 'status'], { stdio: 'ignore' });
  const identities = keychainIdentities();
  requireThat(identities.length, 'No code-signing identity was found in the keychain search list.');
  const prompt = readline.createInterface({ input: process.stdin, output: process.stdout });
  const work = fs.mkdtempSync(path.join(os.tmpdir(), 'passpine-signing-upload-'));
  try {
    identities.forEach((identity, index) => console.log(`${index + 1}) ${identity.name}${identity.status ? ` (${identity.status})` : ''}\n   ${identity.hash}  ${identity.keychain}`));
    const choice = Number(await prompt.question('Select a signing certificate: '));
    const identity = identities[choice - 1];
    requireThat(Number.isInteger(choice) && identity, 'No signing certificate selected.');
    // `security export` cannot pick one identity; keep the full export in a
    // private temporary directory and repack only the selected pair.
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
    const certificate = new crypto.X509Certificate(selected.certificate), fingerprint = certificateFingerprint(certificate.raw);
    if (Date.parse(certificate.validTo) < Date.now()) console.log(`Warning: this certificate expired on ${certificate.validTo}.`);
    const pinned = fs.existsSync(config) ? readSigningConfig(config).certificateSHA256 : undefined;
    console.log(`\nCertificate: ${certificate.subject.replaceAll('\n', ', ')}\nSHA-256:     ${fingerprint}\nRepository:  ${repository}`);
    if (pinned && pinned !== fingerprint) {
      console.log(`This replaces the pinned certificate ${pinned}. Installed clients will see a new app identity once.`);
    }
    const answer = await prompt.question(`Upload ${certificateEnv} and ${passwordEnv}${pinned === fingerprint ? '' : ' and update config/macos-signing.json'}? Type yes to continue: `);
    requireThat(answer.trim() === 'yes', 'Cancelled; nothing was uploaded.');
    execFileSync('gh', ['secret', 'set', certificateEnv, '--repo', repository], { input: fs.readFileSync(files['identity.p12']).toString('base64'), stdio: ['pipe', 'inherit', 'inherit'] });
    execFileSync('gh', ['secret', 'set', passwordEnv, '--repo', repository], { input: password, stdio: ['pipe', 'inherit', 'inherit'] });
    if (pinned !== fingerprint) fs.writeFileSync(config, `${JSON.stringify({ schema: 1, certificateSHA256: fingerprint }, null, 2)}\n`);
    return { fingerprint, changed: pinned !== fingerprint };
  } finally {
    prompt.close();
    fs.rmSync(work, { recursive: true, force: true });
  }
}

if (process.argv[1] && pathToFileURL(path.resolve(process.argv[1])).href === import.meta.url) {
  try {
    const [action, ...args] = process.argv.slice(2);
    if (action === 'generate' && args.length <= 1) {
      const result = generate(args[0] ? { output: path.resolve(args[0]) } : {});
      console.log(`Pinned certificate SHA-256 ${result.fingerprint} in config/macos-signing.json.`);
      console.log(`Store the identity as Actions secrets, then keep ${path.dirname(result.identity)} backed up offline:`);
      console.log(`  base64 < '${result.identity}' | gh secret set ${certificateEnv} --repo knothhe/passpine`);
      console.log(`  gh secret set ${passwordEnv} --repo knothhe/passpine < '${result.password}'`);
    } else if (action === 'verify' && args.length === 1) {
      verify(path.resolve(args[0]));
      console.log(`Verified the pinned release signature: ${args[0]}`);
    } else if (action === 'build' && args.length === 0) {
      buildRelease();
    } else if (action === 'upload' && (args.length === 0 || (args.length === 2 && args[0] === '--repo'))) {
      const result = await uploadKeychainIdentity(args[1] ? { repository: args[1] } : {});
      console.log(result.changed ? `Uploaded the secrets and pinned ${result.fingerprint}; commit config/macos-signing.json before the next release.` : 'Uploaded the secrets for the pinned certificate.');
    } else {
      throw new Error('Usage: macos-signing.mjs generate [OUTPUT_DIR] | upload [--repo OWNER/NAME] | verify APP_BUNDLE | build');
    }
  } catch (error) {
    console.error(`macOS signing failed: ${error.message}`);
    process.exitCode = 1;
  }
}
