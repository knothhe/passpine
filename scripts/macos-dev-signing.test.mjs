import assert from 'node:assert/strict';
import crypto from 'node:crypto';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { spawnSync } from 'node:child_process';
import { test } from 'node:test';
import { fileURLToPath } from 'node:url';
import { discoverDevSigningIdentities, publicSigningCertificate, readDevSigningConfig, rememberDevSigningIdentity, resolveDevSigning, validateDevSigningConfig, writeDevSigningConfig } from './macos-dev-signing.mjs';

// Public certificates only, generated for these tests. Their private keys were
// discarded; the test suite never invokes security or accesses a user keychain.
const signingCertificate = `-----BEGIN CERTIFICATE-----
MIIDNzCCAh+gAwIBAgIUNMcBYCcsHN60m3GfH3gkLf4QtvAwDQYJKoZIhvcNAQEL
BQAwKTEnMCUGA1UEAwweSXNvbGF0ZWQgUGFzc3BpbmUgVGVzdCBzaWduaW5nMB4X
DTI2MTAyODE1NDQ1NFoXDTM2MTAyNTE1NDQ1NFowKTEnMCUGA1UEAwweSXNvbGF0
ZWQgUGFzc3BpbmUgVGVzdCBzaWduaW5nMIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8A
MIIBCgKCAQEAyo4bhJZOUXyB/bBGy22D9BrVzViqWaBd7CH2NlfW3Futl7cNxS8Z
HOL5hwwoHlS6ikTzApmbpHCokxUyPsNzdMTx+AROc97k+A8JaYmJPo+On4dl+eak
Y6mCXpdE1jr2VJMUqA8VVLYCLe12m5asXSQqTQA+5Cj/CYC2Ej3Zt3lrFm9IpffC
fYW0umBjUtG6xuZXQOm1YvYf/ZeWmDzVr4z6ERvHueTVD3Kd3uAzXpXYlOrCS7cG
VpWoSU6Xg8w9YWipwf66b5iBt/ZZb7TxqzMEqWmpPgouZU+TKPoluGR4m55IjJWP
9SEOhW6g3kiHyjKlHHyvUahFXc4mQBb1ZQIDAQABo1cwVTAMBgNVHRMBAf8EAjAA
MA4GA1UdDwEB/wQEAwIHgDAWBgNVHSUBAf8EDDAKBggrBgEFBQcDAzAdBgNVHQ4E
FgQUhg0zdp0Hd5M/44ItjS5Gt8EYC8IwDQYJKoZIhvcNAQELBQADggEBAA7HaDEm
nYXhor/X2Rrici/nSvAQH2dd4zuog92Fofv00sBmZnmcGvA/gUmNR3xYaGNGD12P
x4EKRA0UtACSQj69sKl2iBpySA9ceCcCC5HXfo4k4VCZNiXvly78aB1xWa2Pwr81
HXwHf9dlTiDsU13AmY1hvXDFZMI5u8Jo+TRSOaC0pYmerSv4TPwCFVxgvr8n7JQv
F1C966uvQ0/WvHWyzuPYdIVgI/Jf637erKXk3Rbtz9BxU10JmkJ/OSJen6yeVbp2
JOTyD7LljqW8ZXRrDmbZ4p5ATXp9FkpWbpNiDn8xd9Stn7k4sddcss81ImStu+qQ
7juGMJoRvPwqzzQ=
-----END CERTIFICATE-----`;
const serverCertificate = `-----BEGIN CERTIFICATE-----
MIIDNTCCAh2gAwIBAgIUeUzA8c1JwyXt1m+VFnycT77uwa0wDQYJKoZIhvcNAQEL
BQAwKDEmMCQGA1UEAwwdSXNvbGF0ZWQgUGFzc3BpbmUgVGVzdCBzZXJ2ZXIwHhcN
MjYxMDI4MTU0NDU0WhcNMzYxMDI1MTU0NDU0WjAoMSYwJAYDVQQDDB1Jc29sYXRl
ZCBQYXNzcGluZSBUZXN0IHNlcnZlcjCCASIwDQYJKoZIhvcNAQEBBQADggEPADCC
AQoCggEBAMi6KLine3P+hYUTSR+6h07NT06YtFlPZaagjwSFU+HDqmn2qsdy/0mv
oZ55jnPPHpKiSlERT995GYnf2QDFaJSuTPOXbbhWXkqD8vCB1grZAP5vuy1LRYY2
63ulwasDxSR/0khzXyFEcCeKOORENMbxB2/lH2JE2w1La8/YKp1cxxDvLG6e43Gp
3S6HbEuRqXFeRxVReXH5efvq2df4P7zBHQfXwqe8Guvp+o/hVXxiXMz4Qaz5y14I
Yicl+o44sDsNsUnewjqGmFYkeMbElL6b53ERXDeeTF3vT+4ET0zOPhHGGraezkYQ
+e8d/ElLqbtImiiCXtK65RQUtBiMyQECAwEAAaNXMFUwDAYDVR0TAQH/BAIwADAO
BgNVHQ8BAf8EBAMCB4AwFgYDVR0lAQH/BAwwCgYIKwYBBQUHAwEwHQYDVR0OBBYE
FIUU3Hrai6jzqBxeyv40h+CV/0IHMA0GCSqGSIb3DQEBCwUAA4IBAQCGf332jFac
cibcqFRbM5YZUEkhzrja9R1o1K19lSpO/4qfO5/VcS0/vk01sdF21hEFpoMOpmIc
+ds1LlVQC3vV6SSVqWxPWNePyEfwtd4tV77s+e5D37Nc7tssF5FbLEEmzs5HNdf9
wPw3gifKGcYx2LqzDu4nO+QC6GU81JUwQYs8T8LeGtNEAa//MgRLvpHQtmNi47fB
rzjIdnV7w6Mb5sGuiO0A7gQe/pEmUIkzXIcLEEVPW/iLKYO1b1iyKijzBGOaeIFF
xX4YEZTyLXZ0dVv8cpeUX36jTgdkSotB/o+afvE7euqfcYX/KKXdA9niH30Q0JE6
ksxJJ056+AfP
-----END CERTIFICATE-----`;

const first = { identitySHA1: 'A'.repeat(40), certificateSHA256: 'a'.repeat(64), name: 'Local self-signed', status: 'CSSMERR_TP_NOT_TRUSTED', validTo: '2036-08-26T00:00:00.000Z' };
const second = { ...first, identitySHA1: 'B'.repeat(40), certificateSHA256: 'b'.repeat(64), name: 'Second local identity' };
const record = identity => ({ schema: 1, identitySHA1: identity.identitySHA1, certificateSHA256: identity.certificateSHA256 });
const selected = identity => ({ identitySHA1: identity.identitySHA1, certificateSHA256: identity.certificateSHA256 });
function memoryResolver(overrides = {}) {
  const state = { reads: 0, discoveries: 0, writes: [], choices: 0, output: '', remembered: undefined };
  const options = {
    env: {}, platform: 'darwin', configPath: '/isolated/repository/.macos-dev-signing.json', interactive: false,
    readConfig: () => { state.reads++; return state.remembered; },
    writeConfig: (value, filename) => { state.writes.push({ value, filename }); state.remembered = record(value); },
    discover: () => { state.discoveries++; return [first, second]; },
    choose: async () => { state.choices++; return '2'; },
    output: value => { state.output += value; },
    ...overrides,
  };
  return { state, options };
}

test('discovery reads public identity pairs without hiding untrusted self-signed identities', () => {
  const now = new Date('2030-01-01T00:00:00Z');
  const certificate = publicSigningCertificate(signingCertificate, now);
  const server = publicSigningCertificate(serverCertificate, now);
  const calls = [];
  const result = discoverDevSigningIdentities({ now, run: (program, args) => {
    calls.push([program, args]);
    if (args[0] === 'find-identity') return `  1) ${certificate.identitySHA1} "Local test" (CSSMERR_TP_NOT_TRUSTED)\n  2) ${server.identitySHA1} "Server"\n  3) ${'C'.repeat(40)} "Missing certificate"\n  1) ${certificate.identitySHA1} "Duplicate valid-list entry"\n`;
    return `${signingCertificate}\n${serverCertificate}`;
  } });
  assert.deepEqual(calls, [
    ['/usr/bin/security', ['find-identity', '-p', 'codesigning']],
    ['/usr/bin/security', ['find-certificate', '-a', '-p']],
  ]);
  assert.equal(result.length, 3);
  assert.equal(result[0].status, 'CSSMERR_TP_NOT_TRUSTED');
  assert.equal(result[0].problem, undefined);
  assert.match(result[1].problem, /does not permit code signing/);
  assert.match(result[2].problem, /public certificate is unavailable/);
  assert.equal(result[0].certificateSHA256, crypto.createHash('sha256').update(new crypto.X509Certificate(signingCertificate).raw).digest('hex'));
});

test('certificate checks reject wrong EKU, expired and not-yet-valid certificates', () => {
  assert.match(publicSigningCertificate(serverCertificate, new Date('2030-01-01')).problem, /code signing/);
  assert.match(publicSigningCertificate(signingCertificate, new Date('2040-01-01')).problem, /expired/);
  assert.match(publicSigningCertificate(signingCertificate, new Date('2020-01-01')).problem, /not valid until/);
  assert.throws(() => publicSigningCertificate('invalid certificate'));
});

test('explicit signing keychain scopes both public identity and certificate lookup', async () => {
  const keychain = path.resolve('/isolated/keychain with spaces.keychain-db');
  const calls = [];
  discoverDevSigningIdentities({ keychain, run: (program, args) => { calls.push([program, args]); return ''; } });
  assert.deepEqual(calls, [
    ['/usr/bin/security', ['find-identity', '-p', 'codesigning', keychain]],
    ['/usr/bin/security', ['find-certificate', '-a', '-p', keychain]],
  ]);
  let optionsPassed;
  const fixture = memoryResolver({ env: { PASSPINE_MACOS_SIGNING_IDENTITY: first.identitySHA1, PASSPINE_MACOS_SIGNING_KEYCHAIN: keychain }, discover: options => { optionsPassed = options; return [first]; } });
  await resolveDevSigning(fixture.options);
  assert.deepEqual(optionsPassed, { keychain });
});

test('an explicit exact hash takes precedence without reading or changing the remembered choice', async () => {
  const { state, options } = memoryResolver({ env: { PASSPINE_MACOS_SIGNING_IDENTITY: second.identitySHA1.toLowerCase() } });
  state.remembered = record(first);
  assert.deepEqual(await resolveDevSigning(options), selected(second));
  assert.equal(state.reads, 0);
  assert.equal(state.discoveries, 1);
  assert.equal(state.choices, 0);
  assert.equal(state.writes.length, 0);
  assert.deepEqual(state.remembered, record(first));
});

test('names, ad-hoc and malformed explicit identities stop before keychain inspection', async () => {
  for (const identity of ['-', 'Local self-signed', 'A'.repeat(39), 'A'.repeat(41), `${first.identitySHA1}\n`, 'g'.repeat(40)]) {
    const { state, options } = memoryResolver({ env: { PASSPINE_MACOS_SIGNING_IDENTITY: identity }, interactive: true });
    await assert.rejects(resolveDevSigning(options), /exact 40-digit/);
    assert.equal(state.discoveries, 0);
    assert.equal(state.choices, 0);
    assert.equal(state.writes.length, 0);
  }
});

test('remembered public identity is reused without prompting or persistence', async () => {
  const { state, options } = memoryResolver({ interactive: true });
  state.remembered = record(first);
  assert.deepEqual(await resolveDevSigning(options), selected(first));
  assert.equal(state.choices, 0);
  assert.equal(state.writes.length, 0);
});

test('missing, invalid and replaced remembered certificates never select another identity', async () => {
  for (const identities of [[second], [{ ...first, problem: 'certificate expired on 2020-01-01' }, second], [{ ...first, certificateSHA256: 'c'.repeat(64) }, second]]) {
    const { state, options } = memoryResolver({ interactive: true, discover: () => identities });
    state.remembered = record(first);
    await assert.rejects(resolveDevSigning(options), /missing|expired|no longer matches/);
    assert.equal(state.choices, 0);
    assert.equal(state.writes.length, 0);
    assert.deepEqual(state.remembered, record(first));
  }
});

test('an unusable explicit selection cannot fall back to the remembered identity', async () => {
  const { state, options } = memoryResolver({ env: { PASSPINE_MACOS_SIGNING_IDENTITY: second.identitySHA1 }, discover: () => [first] });
  state.remembered = record(first);
  await assert.rejects(resolveDevSigning(options), /missing or has no paired private key/);
  assert.equal(state.reads, 0);
  assert.equal(state.writes.length, 0);
});

test('first interactive selection displays public metadata and remembers the chosen identity', async () => {
  const { state, options } = memoryResolver({ interactive: true });
  assert.deepEqual(await resolveDevSigning(options), selected(second));
  assert.equal(state.choices, 1);
  assert.deepEqual(state.writes, [{ value: selected(second), filename: options.configPath }]);
  for (const identity of [first, second]) {
    assert.ok(state.output.includes(identity.name));
    assert.ok(state.output.includes(identity.identitySHA1));
    assert.ok(state.output.includes(identity.certificateSHA256));
    assert.ok(state.output.includes(identity.validTo));
  }
  assert.deepEqual(await resolveDevSigning(options), selected(second));
  assert.equal(state.choices, 1, 'later builds must not ask again');
});

test('first noninteractive build fails before discovery and includes real recovery instructions', async () => {
  const { state, options } = memoryResolver();
  await assert.rejects(resolveDevSigning(options), error => {
    assert.match(error.message, /node scripts\/macos-dev-signing\.mjs setup/);
    assert.match(error.message, /Self Signed Root and Code Signing/);
    return true;
  });
  assert.equal(state.discoveries, 0);
  assert.equal(state.writes.length, 0);
});

test('no usable identity gives import and creation guidance without creating any identity', async () => {
  const { state, options } = memoryResolver({ interactive: true, discover: () => [] });
  await assert.rejects(resolveDevSigning(options), /Import Items.*Certificate Assistant/);
  assert.equal(state.choices, 0);
  assert.equal(state.writes.length, 0);
});

test('cancelled or out-of-range selections preserve the current local record', async () => {
  for (const answer of ['', 'q', '0', '-1', '3', '1.5', '1e0', '999999999999999999999']) {
    const { state, options } = memoryResolver({ interactive: true, forceSetup: true, choose: async () => answer });
    state.remembered = record(first);
    await assert.rejects(resolveDevSigning(options), /Cancelled/);
    assert.equal(state.writes.length, 0);
    assert.deepEqual(state.remembered, record(first));
  }
});

test('explicit setup can replace a stale selection without changing any release configuration', async () => {
  const { state, options } = memoryResolver({ interactive: true, forceSetup: true, readConfig: () => { throw new Error('stale malformed record'); } });
  state.remembered = { schema: 999 };
  assert.deepEqual(await resolveDevSigning(options), selected(second));
  assert.deepEqual(state.remembered, record(second));
});

test('named setup validates and persists an exact identity without a terminal', async () => {
  const { state, options } = memoryResolver();
  state.remembered = record(first);
  assert.deepEqual(await rememberDevSigningIdentity(second.identitySHA1.toLowerCase(), options), selected(second));
  assert.deepEqual(state.writes, [{ value: selected(second), filename: options.configPath }]);
  assert.equal(state.reads, 0);
  assert.equal(state.choices, 0);
  for (const invalid of ['', '-', 'Local self-signed']) await assert.rejects(rememberDevSigningIdentity(invalid, options), /exact 40-digit/);
});

test('local configuration strictly contains public hashes and round trips in an isolated directory', () => {
  const directory = fs.mkdtempSync(path.join(os.tmpdir(), 'passpine-local-dev-signing-test-'));
  const filename = path.join(directory, '.macos-dev-signing.json');
  try {
    assert.equal(readDevSigningConfig(filename), undefined);
    writeDevSigningConfig(first, filename);
    assert.deepEqual(readDevSigningConfig(filename), record(first));
    if (process.platform !== 'win32') assert.equal(fs.statSync(filename).mode & 0o777, 0o600);
    writeDevSigningConfig(second, filename);
    assert.deepEqual(readDevSigningConfig(filename), record(second));
    assert.deepEqual(fs.readdirSync(directory), ['.macos-dev-signing.json']);
    for (const invalid of [null, [], {}, { ...record(first), schema: 2 }, { ...record(first), identitySHA1: '-' }, { ...record(first), certificateSHA256: 'A'.repeat(64) }, { ...record(first), privateKey: 'must not be stored' }]) {
      assert.throws(() => validateDevSigningConfig(invalid), /record/);
      fs.writeFileSync(filename, JSON.stringify(invalid));
      assert.throws(() => readDevSigningConfig(filename), /node scripts\/macos-dev-signing\.mjs setup/);
    }
    fs.writeFileSync(filename, '{');
    assert.throws(() => readDevSigningConfig(filename), /local Dev signing record/);
    assert.throws(() => readDevSigningConfig(directory), /Could not read/);
  } finally {
    fs.rmSync(directory, { recursive: true, force: true });
  }
});

test('failed persistence propagates without claiming setup success or changing the previous selection', async () => {
  const { state, options } = memoryResolver({ interactive: true, forceSetup: true, writeConfig: () => { throw new Error('isolated disk failure'); } });
  state.remembered = record(first);
  await assert.rejects(resolveDevSigning(options), /isolated disk failure/);
  assert.deepEqual(state.remembered, record(first));
  assert.ok(!state.output.includes('Remembered'));
});

test('other platforms stop before reading any keychain or local signing configuration', async () => {
  const { state, options } = memoryResolver({ platform: 'win32' });
  await assert.rejects(resolveDevSigning(options), /only be configured on macOS/);
  assert.equal(state.reads, 0);
  assert.equal(state.discoveries, 0);
});

test('CLI usage errors leave stdout empty for callers expecting JSON', () => {
  const script = new URL('./macos-dev-signing.mjs', import.meta.url);
  const result = spawnSync(process.execPath, [fileURLToPath(script), 'resolve', '--unknown'], { encoding: 'utf8' });
  assert.equal(result.status, 1);
  assert.equal(result.stdout, '');
  assert.match(result.stderr, /Usage:/);
});
