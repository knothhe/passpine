import assert from 'node:assert/strict';
import crypto from 'node:crypto';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { test } from 'node:test';
import { execFileSync } from 'node:child_process';
import { assertCertificateRequirement, buildRelease, certificateFingerprint, generate, localIdentity, missingSecrets, parseIdentities, parseIdentityList, readSigningConfig, selectIdentity, setupChoices, verifyApp } from './macos-signing.mjs';

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

test('complete designated requirement binds the expected identifier and actual leaf without bypasses', () => {
  const identifier = 'com.knothhe.passpine.dev', digest = 'a'.repeat(40);
  const id = `identifier "${identifier}"`, pin = `certificate leaf = H"${digest}"`;
  const options = { expectedIdentifier: identifier, expectedCertificateSHA1: digest };
  assertCertificateRequirement(`Executable=/x/Passpine.app/Contents/MacOS/passpine\ndesignated => ${id} and ${pin}\n`, options);
  assertCertificateRequirement(`# designated => (${pin}) and (${id})`, options);
  assertCertificateRequirement(`designated => ${id} and certificate root = H"${digest}"`, { ...options, selfSigned: true });
  for (const expression of [
    `cdhash H"${digest}"`, `${id} and ${pin} or true`, `${id} and (${pin} or true)`, `(${id} and ${pin}) or true`,
    `${id} or ${pin}`, `${id} and ${pin} and true`, `${id} and ${pin}\nor true`,
    `${id} and certificate leaf = H"${'b'.repeat(40)}"`, `identifier "other.app" and ${pin}`,
    `${id} and certificate 1 = H"${digest}"`, `${id} and certificate root = H"${digest}"`,
    `${id} and ${pin} and anchor trusted`, `${id} and ${pin}\ndesignated => true`,
  ]) {
    assert.throws(() => assertCertificateRequirement(`designated => ${expression}`, options), undefined, expression);
  }
  assert.throws(() => assertCertificateRequirement(''), /certificate-based/);
});

test('Apple-issued requirements retain their controlled anchor, certificate class and actual developer identity', () => {
  const identifier = 'com.knothhe.passpine', subject = { OU: 'TEAM123456', CN: 'Apple Development: Isolated Test (TEAM123456)' };
  const options = { expectedIdentifier: identifier, subject };
  const developerID = `identifier "${identifier}" and anchor apple generic and certificate 1[field.1.2.840.113635.100.6.2.6] /* exists */ and certificate leaf[field.1.2.840.113635.100.6.1.13] /* exists */ and certificate leaf[subject.OU] = "TEAM123456"`;
  assertCertificateRequirement(`designated => ${developerID}`, options);
  assertCertificateRequirement(`designated => identifier "${identifier}" and anchor apple generic and certificate leaf[subject.CN] = "${subject.CN}" and certificate 1[field.1.2.840.113635.100.6.2.1] exists`, options);
  for (const expression of [
    developerID.replace('TEAM123456', 'OTHER12345'),
    developerID.replace('anchor apple generic', 'anchor trusted'),
    developerID.replace('1.2.840.113635.100.6.2.6', '1.2.3.4'),
    `${developerID} or anchor apple generic`,
    `identifier "${identifier}" and anchor apple generic and certificate leaf[subject.OU] = "TEAM123456"`,
  ]) assert.throws(() => assertCertificateRequirement(`designated => ${expression}`, options));
  assert.throws(() => assertCertificateRequirement(`designated => ${developerID}`, { ...options, selfSigned: true }));
});

function fakeSignedBundle(root, certificate) {
  const bundle = path.join(root, 'Passpine Dev.app'), executableDir = path.join(bundle, 'Contents/MacOS');
  fs.mkdirSync(executableDir, { recursive: true });
  const bundleID = 'com.knothhe.passpine.dev';
  const targets = new Map([[bundle, bundleID], ...[
    ['passpine', bundleID], ['passpine-host', `${bundleID}.host`], ['passpine-device-unlock', `${bundleID}.device-unlock`],
  ].map(([name, identifier]) => [path.join(executableDir, name), identifier])]);
  for (const target of [...targets.keys()].slice(1)) fs.writeFileSync(target, 'isolated executable fixture');
  return { bundle, bundleID, targets, certificate };
}

function fakeCodesign(fixture, { identifierFor, requirementFor, certificateFor, failTarget } = {}) {
  const calls = [];
  const runCodesign = args => {
    calls.push(args);
    const target = args.at(-1), expectedIdentifier = fixture.targets.get(target);
    assert.ok(expectedIdentifier, `unexpected verification target ${target}`);
    if (target === failTarget) throw new Error('fixture signature verification failed');
    if (args[0] === '--verify') {
      if (args.includes('-R')) assert.match(args[args.indexOf('-R') + 1], /^=identifier /, 'codesign inline requirements need the = prefix to distinguish them from paths');
      return '';
    }
    assert.equal(args[0], '-d');
    if (args.includes('--verbose=4')) return `Executable=${target}\nIdentifier=${identifierFor?.(target) ?? expectedIdentifier}\n`;
    const extract = args.find(arg => arg.startsWith('--extract-certificates='));
    if (extract) {
      fs.writeFileSync(`${extract.slice('--extract-certificates='.length)}0`, (certificateFor?.(target) ?? fixture.certificate).raw);
      return '';
    }
    assert.ok(args.includes('-r-'));
    const digest = fixture.certificate.fingerprint.replaceAll(':', '').toLowerCase();
    return `Executable=${target}\n# designated => ${requirementFor?.(target, expectedIdentifier, digest) ?? `identifier "${expectedIdentifier}" and certificate root = H"${digest}"`}\n`;
  };
  return { calls, runCodesign };
}

test('bundle verification checks certificate pins, stable identifiers and complete requirements for every owned executable', () => {
  const root = temporary();
  try {
    const generated = generate({ output: path.join(root, 'identity'), config: path.join(root, 'signing.json') });
    const fixture = fakeSignedBundle(root, new crypto.X509Certificate(fs.readFileSync(generated.certificate)));
    const commands = fakeCodesign(fixture);
    verifyApp(fixture.bundle, { expectedFingerprint: generated.fingerprint, expectedBundleID: fixture.bundleID, runCodesign: commands.runCodesign });
    assert.deepEqual(commands.calls[0], ['--verify', '--deep', '--strict', fixture.bundle]);
    const evaluations = commands.calls.filter(args => args.includes('-R'));
    assert.equal(evaluations.length, 4);
    const digest = fixture.certificate.fingerprint.replaceAll(':', '').toLowerCase();
    for (const [target, identifier] of fixture.targets) {
      assert.ok(evaluations.some(args => args.at(-1) === target && args[args.indexOf('-R') + 1] === `=identifier "${identifier}" and certificate leaf = H"${digest}"`));
    }
    assert.throws(() => verifyApp(fixture.bundle, { expectedFingerprint: 'invalid', runCodesign: commands.runCodesign }), /fingerprint/);
    assert.throws(() => verifyApp(fixture.bundle, { expectedFingerprint: generated.fingerprint, expectedBundleID: 'dev" or true', runCodesign: commands.runCodesign }), /identifier/);
  } finally {
    fs.rmSync(root, { recursive: true, force: true });
  }
});

test('bundle verification refuses a helper with a different certificate, identifier, requirement or invalid signature', () => {
  const root = temporary();
  try {
    const identities = [0, 1].map(index => generate({ output: path.join(root, `identity-${index}`), config: path.join(root, `${index}.json`) }));
    const certificates = identities.map(identity => new crypto.X509Certificate(fs.readFileSync(identity.certificate)));
    const fixture = fakeSignedBundle(root, certificates[0]);
    const host = path.join(fixture.bundle, 'Contents/MacOS/passpine-host');
    const options = { expectedFingerprint: identities[0].fingerprint, expectedBundleID: fixture.bundleID };
    const mutations = [
      [{ certificateFor: target => target === host ? certificates[1] : certificates[0] }, /expected pinned/],
      [{ identifierFor: target => target === host ? 'passpine-host-build-specific' : fixture.targets.get(target) }, /code identifier/],
      [{ requirementFor: (target, identifier, digest) => `identifier "${identifier}" and certificate leaf = H"${digest}"${target === host ? ' or true' : ''}` }, /alternatives/],
      [{ requirementFor: (target, identifier, digest) => `identifier "${identifier}" and certificate leaf = H"${target === host ? 'f'.repeat(40) : digest}"` }, /actual signing certificate/],
      [{ failTarget: host }, /signature verification failed/],
    ];
    for (const [mutation, message] of mutations) {
      const { runCodesign } = fakeCodesign(fixture, mutation);
      assert.throws(() => verifyApp(fixture.bundle, { ...options, runCodesign }), message);
    }
    const { runCodesign } = fakeCodesign(fixture);
    fs.writeFileSync(path.join(fixture.bundle, 'Contents/MacOS/unverified-helper'), 'unexpected helper');
    assert.throws(() => verifyApp(fixture.bundle, { ...options, runCodesign }), /exactly the expected/);
    fs.rmSync(path.join(fixture.bundle, 'Contents/MacOS/unverified-helper'));
    fs.rmSync(host);
    assert.throws(() => verifyApp(fixture.bundle, { ...options, runCodesign }), /exactly the expected/);
  } finally {
    fs.rmSync(root, { recursive: true, force: true });
  }
});

// Windows checkouts convert workflows to CRLF, which the job and step
// boundaries below must not depend on.
const assertSigningScope = (ciSource, releaseSource) => {
  const [ci, release] = [ciSource, releaseSource].map(source => source.replace(/\r\n/g, '\n'));
  const steps = ci.split(/\n(?=      - name: )/);
  const users = steps.filter(step => /secrets\.PASSPINE_MACOS_SIGNING/.test(step));
  assert.equal(users.length, 1);
  assert.match(users[0], /^ +if: runner\.os == 'macOS'$/m);
  assert.match(users[0], /^ +run: node scripts\/macos-signing\.mjs build$/m);
  assert.match(users[0], /^ +PASSPINE_MACOS_VERIFY_DEV_INSTALL: '1'$/m);
  assert.ok(!steps.some(step => /bash scripts\/build-client\.sh/.test(step) && /runner\.os != 'Windows'|runner\.os == 'macOS'/.test(step)), 'macOS release packages must use the signed build.');
  const directAcceptance = steps.filter(step => /run: node scripts\/verify-dev-installation\.mjs --run/.test(step));
  assert.equal(directAcceptance.length, 1);
  assert.match(directAcceptance[0], /name: Verify isolated Windows Dev install and uninstall\n +if: runner\.os == 'Windows'/);
  assert.ok(steps.some(step => /name: Verify isolated Linux Dev install and uninstall\n +if: runner\.os == 'Linux'/.test(step) && /xvfb-run --auto-servernum node scripts\/verify-dev-installation\.mjs --run/.test(step)), 'Linux native acceptance must remain enabled.');
  const jobs = release.split(/\n(?=  [a-z]+:\n)/);
  assert.deepEqual(jobs.filter(job => /PASSPINE_MACOS_SIGNING/.test(job)).map(job => job.trim().split(':')[0]), ['build']);
};

test('the macOS signing identity reaches only the signed macOS build step', () => {
  const ci = fs.readFileSync(new URL('../.github/workflows/ci.yml', import.meta.url), 'utf8');
  const release = fs.readFileSync(new URL('../.github/workflows/release.yml', import.meta.url), 'utf8');
  assertSigningScope(ci, release);
  const crlf = source => source.replace(/\r?\n/g, '\r\n');
  assertSigningScope(crlf(ci), crlf(release));
});

function releaseBuildFixture(root, options = {}) {
  const { failProcess, failSecurity = [], recordAcceptanceLogs = false } = options;
  const acceptanceFlag = Object.hasOwn(options, 'acceptanceFlag') ? options.acceptanceFlag : '1';
  const runner = path.join(root, 'runner temp'), originalKeychain = path.join(root, 'original login.keychain-db');
  fs.mkdirSync(runner);
  fs.mkdirSync(path.join(root, 'config'));
  fs.writeFileSync(path.join(root, 'config/macos-signing.json'), JSON.stringify({ schema: 1, certificateSHA256: 'a'.repeat(64) }));
  fs.writeFileSync(originalKeychain, 'original keychain fixture');
  const env = {
    GITHUB_ACTIONS: 'true', RUNNER_TEMP: runner, PASSPINE_BUILD_CHANNEL: 'release',
    PASSPINE_MACOS_SIGNING_CERTIFICATE: Buffer.from('isolated PKCS12 fixture').toString('base64'),
    PASSPINE_MACOS_SIGNING_CERTIFICATE_PASSWORD: 'isolated password fixture',
  };
  if (acceptanceFlag !== undefined) env.PASSPINE_MACOS_VERIFY_DEV_INSTALL = acceptanceFlag;
  const events = [], processes = [], acceptanceLog = path.join(runner, 'passpine-dev-acceptance-fixture', 'native.log');
  let keychain, identity;
  const runSecurity = args => {
    let event = args[0];
    if (event === 'list-keychains') {
      event = args.includes('-s') ? args[4] === originalKeychain ? 'restore' : 'scope' : 'original';
    }
    events.push(event);
    if (failSecurity.includes(event) && event !== 'create-keychain') throw new Error(`fixture ${event} failure`);
    if (event === 'original') return `    "${originalKeychain}"\n`;
    if (event === 'create-keychain') {
      keychain = args.at(-1);
      fs.writeFileSync(keychain, 'temporary keychain fixture', { mode: 0o600 });
      if (failSecurity.includes(event)) throw new Error('fixture create-keychain failure after partial creation');
    }
    if (event === 'import') {
      identity = args[1];
      assert.equal(fs.readFileSync(identity, 'utf8'), 'isolated PKCS12 fixture');
      if (process.platform !== 'win32') assert.equal(fs.statSync(identity).mode & 0o777, 0o600);
    }
    if (event === 'find-identity') return `  1) ${'A'.repeat(40)} "Isolated signing identity"\n`;
    if (event === 'restore') assert.deepEqual(args, ['list-keychains', '-d', 'user', '-s', originalKeychain]);
    if (event === 'delete-keychain') fs.rmSync(keychain);
    return '';
  };
  const runProcess = (file, args, options) => {
    const operation = file === 'bash' ? 'build' : 'acceptance';
    events.push(operation);
    processes.push({ file, args, ...options });
    assert.equal(options.cwd, root);
    assert.equal(options.stdio, 'inherit', 'native build and acceptance diagnostics must remain visible');
    assert.equal(options.env.PASSPINE_MACOS_SIGNING_IDENTITY, 'A'.repeat(40));
    assert.equal(options.env.PASSPINE_MACOS_SIGNING_KEYCHAIN, keychain);
    assert.equal(Object.hasOwn(options.env, 'PASSPINE_MACOS_SIGNING_CERTIFICATE'), false);
    assert.equal(Object.hasOwn(options.env, 'PASSPINE_MACOS_SIGNING_CERTIFICATE_PASSWORD'), false);
    assert.ok(fs.existsSync(keychain), 'the signing keychain must remain alive during both child processes');
    assert.ok(fs.existsSync(identity));
    if (operation === 'acceptance' && recordAcceptanceLogs) {
      fs.mkdirSync(path.dirname(acceptanceLog));
      fs.writeFileSync(acceptanceLog, 'isolated native lifecycle diagnostics');
    }
    return { status: operation === failProcess ? 23 : 0 };
  };
  return { env, runner, originalKeychain, events, processes, acceptanceLog, run: () => buildRelease(env, root, { platform: 'darwin', runSecurity, runProcess }) };
}

test('CI Dev acceptance shares the imported identity lifetime and stripped build environment', () => {
  const root = temporary();
  try {
    const fixture = releaseBuildFixture(root, { recordAcceptanceLogs: true });
    fixture.run();
    assert.deepEqual(fixture.processes.map(({ file, args }) => [file, args]), [
      ['bash', [path.join(root, 'scripts/build-client.sh')]],
      [process.execPath, [path.join(root, 'scripts/verify-dev-installation.mjs'), '--run']],
    ]);
    assert.equal(fixture.processes[0].env, fixture.processes[1].env);
    assert.ok(fixture.events.indexOf('build') < fixture.events.indexOf('acceptance'));
    assert.ok(fixture.events.indexOf('acceptance') < fixture.events.indexOf('restore'));
    assert.ok(fixture.events.indexOf('restore') < fixture.events.indexOf('delete-keychain'));
    assert.deepEqual(fs.readdirSync(fixture.runner), ['passpine-dev-acceptance-fixture']);
    assert.equal(fs.readFileSync(fixture.acceptanceLog, 'utf8'), 'isolated native lifecycle diagnostics');
    assert.equal(fs.readFileSync(fixture.originalKeychain, 'utf8'), 'original keychain fixture');
    assert.equal(fixture.env.PASSPINE_MACOS_SIGNING_CERTIFICATE_PASSWORD, 'isolated password fixture');
  } finally {
    fs.rmSync(root, { recursive: true, force: true });
  }
});

test('CI Dev acceptance requires the explicit opt-in flag', () => {
  for (const acceptanceFlag of [undefined, '0', 'true']) {
    const root = temporary();
    try {
      const fixture = releaseBuildFixture(root, { acceptanceFlag });
      fixture.run();
      assert.equal(fixture.processes.length, 1);
      assert.equal(fixture.events.includes('acceptance'), false);
      assert.deepEqual(fs.readdirSync(fixture.runner), []);
    } finally {
      fs.rmSync(root, { recursive: true, force: true });
    }
  }
});

test('failed release builds and Dev acceptance propagate failure and still remove the imported material', () => {
  for (const failProcess of ['build', 'acceptance']) {
    const root = temporary();
    try {
      const fixture = releaseBuildFixture(root, { failProcess, recordAcceptanceLogs: true });
      assert.throws(fixture.run, failProcess === 'build' ? /Signed macOS build failed \(23\)/ : /Dev installation acceptance failed \(23\)/);
      assert.equal(fixture.events.includes('acceptance'), failProcess !== 'build');
      assert.ok(fixture.events.includes('restore'));
      assert.ok(fixture.events.includes('delete-keychain'));
      assert.deepEqual(fs.readdirSync(fixture.runner), failProcess === 'acceptance' ? ['passpine-dev-acceptance-fixture'] : []);
      if (failProcess === 'acceptance') assert.equal(fs.readFileSync(fixture.acceptanceLog, 'utf8'), 'isolated native lifecycle diagnostics');
    } finally {
      fs.rmSync(root, { recursive: true, force: true });
    }
  }
});

test('keychain failures cannot skip independent cleanup or leave temporary PKCS12 material behind', () => {
  for (const failSecurity of [['original'], ['create-keychain'], ['import'], ['restore', 'delete-keychain']]) {
    const root = temporary();
    try {
      const fixture = releaseBuildFixture(root, { failSecurity });
      assert.throws(fixture.run, /fixture .* failure/);
      if (!failSecurity.includes('original')) {
        assert.ok(fixture.events.includes('restore'));
        assert.ok(fixture.events.includes('delete-keychain'));
      }
      assert.deepEqual(fs.readdirSync(fixture.runner), []);
      assert.equal(fs.readFileSync(fixture.originalKeychain, 'utf8'), 'original keychain fixture');
    } finally {
      fs.rmSync(root, { recursive: true, force: true });
    }
  }
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
    // A local identity that differs from the pin is offered only as a replacement.
    const other = localIdentity(output, 'f'.repeat(64));
    assert.equal(other.pinned, false);
    const replacing = setupChoices({ pinned: 'f'.repeat(64), local: other, platform: 'linux' });
    assert.deepEqual(replacing.map(choice => choice.action), ['reupload', 'generate']);
    assert.match(replacing[0].label, /replace the pinned one$/);
    assert.match(setupChoices({ pinned: generated.fingerprint, local, missing: ['PASSPINE_MACOS_SIGNING_CERTIFICATE'], platform: 'linux' })[0].label, /restores the missing secrets/);
    assert.doesNotMatch(setupChoices({ pinned: generated.fingerprint, local, platform: 'linux' })[0].label, /missing|replace/);
    assert.match(setupChoices({ pinned: 'f'.repeat(64), platform: 'linux' })[0].label, /replace the pinned/);
    fs.rmSync(local.password);
    assert.equal(localIdentity(output), undefined);
  } finally {
    fs.rmSync(root, { recursive: true, force: true });
  }
});

test('secret presence check names each missing signing secret', () => {
  const both = ['PASSPINE_MACOS_SIGNING_CERTIFICATE', 'PASSPINE_MACOS_SIGNING_CERTIFICATE_PASSWORD'];
  assert.deepEqual(missingSecrets(['HOMEBREW_TAP_SSH_KEY', ...both]), []);
  assert.deepEqual(missingSecrets(['PASSPINE_MACOS_SIGNING_CERTIFICATE']), ['PASSPINE_MACOS_SIGNING_CERTIFICATE_PASSWORD']);
  assert.deepEqual(missingSecrets([]), both);
});
