#!/usr/bin/env node
// Local Dev identity selection only. Private keys stay in the keychain; the
// ignored repository record contains public identity references, never P12s.
import crypto from 'node:crypto';
import fs from 'node:fs';
import path from 'node:path';
import { execFileSync } from 'node:child_process';
import { fileURLToPath, pathToFileURL } from 'node:url';
import readline from 'node:readline/promises';
import { parseIdentityList } from './macos-signing.mjs';

export const defaultDevConfigPath = fileURLToPath(new URL('../.macos-dev-signing.json', import.meta.url));
const identityEnv = 'PASSPINE_MACOS_SIGNING_IDENTITY';
const codeSigningOID = '1.3.6.1.5.5.7.3.3';
const setupCommand = 'node scripts/macos-dev-signing.mjs setup';
const guidance = `Run \`${setupCommand}\` in a terminal to choose or reselect an existing code-signing identity. To import an existing identity, use Keychain Access → File → Import Items. To create a local identity, open Keychain Access → Certificate Assistant → Create a Certificate; select Self Signed Root and Code Signing, set a suitable validity period, and keep its paired private key in the login keychain. Then run the setup command again. No Apple Developer purchase or release Actions secrets are required.`;
const requireThat = (condition, message) => { if (!condition) throw new Error(message); };
const display = value => String(value).replace(/[\u0000-\u001f\u007f]/g, ' ');

export function validateDevSigningConfig(value) {
  requireThat(value && typeof value === 'object' && !Array.isArray(value) && Object.keys(value).sort().join(',') === 'certificateSHA256,identitySHA1,schema', 'The local Dev signing record must contain only schema, identitySHA1, and certificateSHA256.');
  requireThat(value.schema === 1 && typeof value.identitySHA1 === 'string' && /^[A-F0-9]{40}$/.test(value.identitySHA1) && typeof value.certificateSHA256 === 'string' && /^[a-f0-9]{64}$/.test(value.certificateSHA256), 'Invalid local Dev signing record.');
  return value;
}

export function readDevSigningConfig(filename = defaultDevConfigPath) {
  let content;
  try {
    content = fs.readFileSync(filename, 'utf8');
  } catch (error) {
    if (error.code === 'ENOENT') return undefined;
    throw new Error(`Could not read the local Dev signing record. ${guidance}`, { cause: error });
  }
  try {
    return validateDevSigningConfig(JSON.parse(content));
  } catch (error) {
    throw new Error(`Could not use the local Dev signing record: ${error.message} ${guidance}`, { cause: error });
  }
}

export function writeDevSigningConfig(identity, filename = defaultDevConfigPath) {
  const record = validateDevSigningConfig({ schema: 1, identitySHA1: identity.identitySHA1, certificateSHA256: identity.certificateSHA256 });
  const temporary = path.join(path.dirname(filename), `.macos-dev-signing-${crypto.randomBytes(12).toString('hex')}.tmp`);
  let descriptor;
  try {
    descriptor = fs.openSync(temporary, 'wx', 0o600);
    fs.writeFileSync(descriptor, `${JSON.stringify(record, null, 2)}\n`);
    fs.fsyncSync(descriptor);
    fs.closeSync(descriptor);
    descriptor = undefined;
    fs.renameSync(temporary, filename);
  } finally {
    if (descriptor !== undefined) fs.closeSync(descriptor);
    fs.rmSync(temporary, { force: true });
  }
}

export function publicSigningCertificate(pem, now = new Date()) {
  const certificate = new crypto.X509Certificate(pem);
  const validFrom = new Date(certificate.validFrom), validTo = new Date(certificate.validTo);
  let problem;
  if (!certificate.keyUsage?.includes(codeSigningOID)) problem = 'certificate does not permit code signing';
  else if (validFrom > now) problem = `certificate is not valid until ${validFrom.toISOString()}`;
  else if (validTo <= now) problem = `certificate expired on ${validTo.toISOString()}`;
  return {
    identitySHA1: certificate.fingerprint.replaceAll(':', ''),
    certificateSHA256: crypto.createHash('sha256').update(certificate.raw).digest('hex'),
    validFrom: validFrom.toISOString(), validTo: validTo.toISOString(), problem,
  };
}

function runSecurity(program, args) {
  try {
    return execFileSync(program, args, { encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'] });
  } catch (error) {
    throw new Error(`Could not inspect public macOS code-signing identities. ${guidance}`, { cause: error });
  }
}

export function discoverDevSigningIdentities({ run = runSecurity, now = new Date(), keychain } = {}) {
  requireThat(keychain === undefined || (typeof keychain === 'string' && keychain.length > 0 && !keychain.includes('\0')), 'The explicit macOS signing keychain path is invalid.');
  const scope = keychain === undefined ? [] : [path.resolve(keychain)];
  // find-identity lists certificates with paired private keys. Do
  // not use -v: it would hide usable, untrusted local self-signed identities.
  const identities = parseIdentityList(run('/usr/bin/security', ['find-identity', '-p', 'codesigning', ...scope]));
  const publicPEM = run('/usr/bin/security', ['find-certificate', '-a', '-p', ...scope]);
  const certificates = new Map();
  for (const match of publicPEM.matchAll(/-----BEGIN CERTIFICATE-----[\s\S]+?-----END CERTIFICATE-----/g)) {
    const certificate = publicSigningCertificate(match[0], now);
    certificates.set(certificate.identitySHA1, certificate);
  }
  return identities.map(identity => ({
    ...(certificates.get(identity.hash) ?? { identitySHA1: identity.hash, problem: 'matching public certificate is unavailable' }),
    name: identity.name, status: identity.status,
  }));
}

function selectedIdentity(identities, identitySHA1, expectedSHA256) {
  const identity = identities.find(candidate => candidate.identitySHA1 === identitySHA1);
  requireThat(identity, `The selected local Dev signing identity ${identitySHA1} is missing or has no paired private key. ${guidance}`);
  requireThat(!identity.problem, `The selected local Dev signing identity ${identitySHA1} cannot be used: ${identity.problem}. ${guidance}`);
  requireThat(/^[a-f0-9]{64}$/.test(identity.certificateSHA256), `The selected identity has no valid public certificate fingerprint. ${guidance}`);
  requireThat(!expectedSHA256 || identity.certificateSHA256 === expectedSHA256, `The selected identity no longer matches the certificate remembered for this repository. ${guidance}`);
  return { identitySHA1, certificateSHA256: identity.certificateSHA256 };
}

async function terminalChoice(question) {
  const terminal = readline.createInterface({ input: process.stdin, output: process.stderr });
  try {
    return await terminal.question(question);
  } catch (error) {
    if (error.code === 'ABORT_ERR' || error.code === 'ERR_USE_AFTER_CLOSE') throw new Error('Cancelled; the local Dev signing selection was not changed.');
    throw error;
  } finally {
    terminal.close();
  }
}

export async function resolveDevSigning({
  env = process.env, configPath = defaultDevConfigPath, platform = process.platform,
  discover = discoverDevSigningIdentities, readConfig = readDevSigningConfig, writeConfig = writeDevSigningConfig,
  interactive = Boolean(process.stdin.isTTY && process.stderr.isTTY), choose = terminalChoice,
  output = message => process.stderr.write(message), forceSetup = false,
} = {}) {
  requireThat(platform === 'darwin', 'Local macOS Dev signing can only be configured on macOS.');
  // Explicit setup deliberately reopens selection, including recovery from an
  // invalid old record. Ordinary resolve always honors the one-shot override.
  const explicit = forceSetup ? undefined : env[identityEnv];
  const discoveryOptions = { keychain: env.PASSPINE_MACOS_SIGNING_KEYCHAIN || undefined };
  if (explicit !== undefined && explicit !== '') {
    requireThat(typeof explicit === 'string' && /^[a-fA-F0-9]{40}$/.test(explicit), `${identityEnv} must be the exact 40-digit certificate identity hash; names and ad-hoc signing are not accepted. ${guidance}`);
    return selectedIdentity(await discover(discoveryOptions), explicit.toUpperCase());
  }
  const remembered = forceSetup ? undefined : await readConfig(configPath);
  if (remembered) {
    validateDevSigningConfig(remembered);
    return selectedIdentity(await discover(discoveryOptions), remembered.identitySHA1, remembered.certificateSHA256);
  }
  requireThat(interactive, `No local Dev signing identity is selected. ${guidance}`);
  const identities = await discover(discoveryOptions);
  const usable = identities.filter(identity => !identity.problem);
  requireThat(usable.length, `No usable code-signing identity with a valid certificate and paired private key was found. ${guidance}`);
  output('Passpine macOS Dev signing — choose a fixed local identity.\n');
  usable.forEach((identity, index) => output(`${index + 1}) ${display(identity.name)}${identity.status ? ` (${display(identity.status)})` : ''}\n   SHA-1: ${identity.identitySHA1}\n   SHA-256: ${identity.certificateSHA256}\n   Expires: ${identity.validTo}\n`));
  output('q) Cancel\n');
  const answer = String(await choose('Select a signing identity: ')).trim();
  const index = /^\d+$/.test(answer) ? Number(answer) - 1 : -1;
  requireThat(Number.isSafeInteger(index) && usable[index], 'Cancelled; the local Dev signing selection was not changed.');
  const result = selectedIdentity(usable, usable[index].identitySHA1);
  await writeConfig(result, configPath);
  output(`Remembered the public Dev signing identity in ${display(configPath)}.\n`);
  return result;
}

// A named setup selection is an explicit request to remember this identity;
// an ordinary environment override remains temporary.
export async function rememberDevSigningIdentity(identitySHA1, options = {}) {
  requireThat(typeof identitySHA1 === 'string' && /^[a-fA-F0-9]{40}$/.test(identitySHA1), 'setup --identity requires an exact 40-digit certificate identity hash.');
  const result = await resolveDevSigning({ ...options, env: { ...(options.env ?? process.env), [identityEnv]: identitySHA1 }, forceSetup: false });
  await (options.writeConfig ?? writeDevSigningConfig)(result, options.configPath ?? defaultDevConfigPath);
  return result;
}

if (process.argv[1] && pathToFileURL(path.resolve(process.argv[1])).href === import.meta.url) {
  try {
    const [action, ...args] = process.argv.slice(2);
    const namedSetup = action === 'setup' && args.length === 2 && args[0] === '--identity';
    requireThat(namedSetup || ((action === 'resolve' || action === 'setup') && args.length === 0), 'Usage: node scripts/macos-dev-signing.mjs resolve | setup [--identity SHA1]');
    const identity = namedSetup ? await rememberDevSigningIdentity(args[1]) : await resolveDevSigning({ forceSetup: action === 'setup' });
    process.stdout.write(`${JSON.stringify(identity)}\n`);
  } catch (error) {
    process.stderr.write(`macOS Dev signing failed: ${error.message}\n`);
    process.exitCode = 1;
  }
}
