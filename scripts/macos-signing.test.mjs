import assert from 'node:assert/strict';
import crypto from 'node:crypto';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { test } from 'node:test';
import { execFileSync } from 'node:child_process';
import { assertCertificateRequirement, certificateFingerprint, generate, localIdentity, parseIdentities, parseIdentityList, readSigningConfig, selectIdentity, setupChoices } from './macos-signing.mjs';

const temporary = () => fs.mkdtempSync(path.join(os.tmpdir(), 'passpine-macos-signing-test-'));

test('generated identity is a pinned code-signing certificate that is never replaced', () => {
  const root = temporary();
  try {
    const output = path.join(root, 'identity dir'), config = path.join(root, 'macos-signing.json');
    const result = generate({ output, config });
    const certificate = new crypto.X509Certificate(fs.readFileSync(result.certificate));
    assert.ok(certificate.keyUsage.includes('1.3.6.1.5.5.7.3.3'));
    assert.equal(certificate.ca, false);
    assert.equal(certificateFingerprint(fs.readFileSync(result.certificate)), result.fingerprint);
    assert.deepEqual(readSigningConfig(config), { schema: 1, certificateSHA256: result.fingerprint });
    assert.ok(fs.statSync(result.identity).size > 0);
    if (process.platform !== 'win32') {
      assert.equal(fs.statSync(output).mode & 0o777, 0o700);
      for (const file of [result.certificate, result.identity, result.password]) assert.equal(fs.statSync(file).mode & 0o777, 0o600);
    }
    const before = fs.readFileSync(config, 'utf8');
    assert.throws(() => generate({ output, config }), /already exists/);
    fs.rmSync(config);
    assert.throws(() => generate({ output, config }), /already exists/);
    assert.equal(fs.existsSync(config), false);
    assert.ok(before.includes(result.fingerprint));
  } finally {
    fs.rmSync(root, { recursive: true, force: true });
  }
});

test('signing configuration accepts only the pinned certificate field', () => {
  const root = temporary();
  try {
    const config = path.join(root, 'macos-signing.json');
    assert.throws(() => readSigningConfig(config), /not configured/);
    for (const value of [{ schema: 1 }, { schema: 2, certificateSHA256: 'a'.repeat(64) }, { schema: 1, certificateSHA256: 'A'.repeat(64) }, { schema: 1, certificateSHA256: 'a'.repeat(64), password: 'x' }]) {
      fs.writeFileSync(config, JSON.stringify(value));
      assert.throws(() => readSigningConfig(config));
    }
  } finally {
    fs.rmSync(root, { recursive: true, force: true });
  }
});

test('identity listing includes untrusted self-signed identities once', () => {
  const hash = 'ABCDEF0123456789ABCDEF0123456789ABCDEF01';
  const output = `Policy: Code Signing\n  Matching identities\n  1) ${hash} "Passpine Release Signing (self-signed)" (CSSMERR_TP_NOT_TRUSTED)\n     1 identities found\n\n  Valid identities only\n     0 valid identities found\n`;
  assert.deepEqual(parseIdentities(output), [hash]);
  assert.deepEqual(parseIdentities(`${output}  1) ${hash} "Passpine"\n`), [hash]);
  assert.deepEqual(parseIdentities('     0 identities found\n'), []);
});

test('designated requirement must pin the leaf certificate instead of a build hash', () => {
  assertCertificateRequirement(`Executable=/x/Passpine.app/Contents/MacOS/passpine\ndesignated => identifier "com.knothhe.passpine" and certificate leaf = H"${'a'.repeat(40)}"\n`);
  assert.throws(() => assertCertificateRequirement(`designated => cdhash H"${'a'.repeat(40)}"\n`), /certificate-based/);
  assert.throws(() => assertCertificateRequirement(''), /certificate-based/);
});

test('the macOS signing identity reaches only the signed macOS build step', () => {
  const ci = fs.readFileSync(new URL('../.github/workflows/ci.yml', import.meta.url), 'utf8');
  const release = fs.readFileSync(new URL('../.github/workflows/release.yml', import.meta.url), 'utf8');
  const steps = ci.split(/\n(?=      - name: )/);
  const users = steps.filter(step => /secrets\.PASSPINE_MACOS_SIGNING/.test(step));
  assert.equal(users.length, 1);
  assert.match(users[0], /^ +if: runner\.os == 'macOS'$/m);
  assert.match(users[0], /^ +run: node scripts\/macos-signing\.mjs build$/m);
  assert.ok(!steps.some(step => /bash scripts\/build-client\.sh/.test(step) && /runner\.os != 'Windows'|runner\.os == 'macOS'/.test(step)), 'macOS release packages must use the signed build.');
  const jobs = release.split(/\n(?=  [a-z]+:\n)/);
  assert.deepEqual(jobs.filter(job => /PASSPINE_MACOS_SIGNING/.test(job)).map(job => job.trim().split(':')[0]), ['build']);
});

test('identity list keeps names, trust status and keychain order', () => {
  const [first, second] = ['A'.repeat(40), 'B'.repeat(40)];
  const output = `  1) ${first} "Apple Development: Name (TEAM)"\n  2) ${second} "Passpine (self-signed)" (CSSMERR_TP_NOT_TRUSTED)\n     2 identities found\n\n  Valid identities only\n  1) ${first} "Apple Development: Name (TEAM)"\n`;
  assert.deepEqual(parseIdentityList(output), [
    { hash: first, name: 'Apple Development: Name (TEAM)', status: '' },
    { hash: second, name: 'Passpine (self-signed)', status: 'CSSMERR_TP_NOT_TRUSTED' },
  ]);
});

test('keychain export selection repacks only the chosen certificate and its key', () => {
  const root = temporary();
  try {
    const identities = [0, 1].map(index => generate({ output: path.join(root, String(index)), config: path.join(root, `${index}.json`) }));
    const keyFor = index => {
      // Recover each generated key through the same PKCS#12 dump openssl prints.
      const dump = execFileSync('openssl', ['pkcs12', '-in', identities[index].identity, '-nodes', '-passin', `file:${identities[index].password}`], { encoding: 'utf8' });
      return dump.match(/-----BEGIN PRIVATE KEY-----[\s\S]+?-----END PRIVATE KEY-----/)[0];
    };
    const certificates = identities.map(identity => new crypto.X509Certificate(fs.readFileSync(identity.certificate)));
    const dump = `Bag Attributes\n${keyFor(0)}\n${certificates[0].toString()}\nBag Attributes\n${certificates[1].toString()}\n${keyFor(1)}\n`;
    const sha1 = certificates[1].fingerprint.replaceAll(':', '').toLowerCase();
    const selected = selectIdentity(dump, sha1);
    assert.equal(new crypto.X509Certificate(selected.certificate).fingerprint256, certificates[1].fingerprint256);
    assert.equal(crypto.createPublicKey(selected.key).export({ type: 'spki', format: 'der' }).toString('hex'), certificates[1].publicKey.export({ type: 'spki', format: 'der' }).toString('hex'));
    assert.throws(() => selectIdentity(`${keyFor(0)}\n${certificates[1].toString()}`, sha1), /private key/);
    assert.throws(() => selectIdentity(dump, 'C'.repeat(40)), /missing from the keychain export/);
  } finally {
    fs.rmSync(root, { recursive: true, force: true });
  }
});

test('forced generation replaces the pin and keeps the previous identity', () => {
  const root = temporary();
  try {
    const output = path.join(root, 'identity'), config = path.join(root, 'macos-signing.json');
    const first = generate({ output, config });
    const previous = fs.readFileSync(first.identity);
    assert.throws(() => generate({ output, config }), /reuse or replace it/);
    const second = generate({ output, config, force: true, now: new Date('2026-10-25T01:02:03.004Z') });
    assert.notEqual(second.fingerprint, first.fingerprint);
    assert.equal(readSigningConfig(config).certificateSHA256, second.fingerprint);
    assert.equal(second.replaced, path.join(output, 'replaced-2026-10-25T01-02-03-004Z'));
    assert.deepEqual(fs.readFileSync(path.join(second.replaced, 'identity.p12')), previous);
    if (process.platform !== 'win32') assert.equal(fs.statSync(second.replaced).mode & 0o777, 0o700);
    // A pinned repository config alone, as on a fresh checkout, is also replaceable.
    const fresh = generate({ output: path.join(root, 'fresh'), config, force: true });
    assert.equal(fresh.replaced, undefined);
    assert.equal(readSigningConfig(config).certificateSHA256, fresh.fingerprint);
  } finally {
    fs.rmSync(root, { recursive: true, force: true });
  }
});

test('setup offers only the actions that fit the pinned and local identities', () => {
  const root = temporary();
  try {
    const output = path.join(root, 'identity'), config = path.join(root, 'macos-signing.json');
    assert.equal(localIdentity(output), undefined);
    const generated = generate({ output, config });
    const local = localIdentity(output, generated.fingerprint);
    assert.equal(local.fingerprint, generated.fingerprint);
    assert.equal(local.pinned, true);
    const actions = options => setupChoices(options).map(choice => choice.action);
    assert.deepEqual(actions({ platform: 'linux' }), ['generate']);
    assert.deepEqual(actions({ platform: 'darwin' }), ['keychain', 'generate']);
    assert.deepEqual(actions({ pinned: generated.fingerprint, local, platform: 'darwin' }), ['reupload', 'keychain', 'generate']);
    assert.deepEqual(actions({ local: { ...local, pinned: false }, platform: 'linux' }), ['reupload', 'generate']);
    // A local identity that differs from the pin must not be uploaded silently.
    const other = localIdentity(output, 'f'.repeat(64));
    assert.equal(other.pinned, false);
    assert.deepEqual(actions({ pinned: 'f'.repeat(64), local: other, platform: 'linux' }), ['generate']);
    assert.match(setupChoices({ pinned: 'f'.repeat(64), platform: 'linux' })[0].label, /replace the pinned/);
    fs.rmSync(local.password);
    assert.equal(localIdentity(output), undefined);
  } finally {
    fs.rmSync(root, { recursive: true, force: true });
  }
});
