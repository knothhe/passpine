import assert from 'node:assert/strict';
import crypto from 'node:crypto';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { test } from 'node:test';
import { assertCertificateRequirement, certificateFingerprint, generate, parseIdentities, readSigningConfig } from './macos-signing.mjs';

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
