import { afterEach, expect, it, vi } from 'vitest';

afterEach(() => { vi.unstubAllGlobals(); vi.unstubAllEnvs(); vi.resetModules(); });

it('direct source defaults to an isolated development identity and bilingual copy', async () => {
  vi.stubEnv('PASSPINE_BUILD_CHANNEL', '');
  vi.resetModules();
  const { BUILD_CHANNEL } = await import('./build-channel');
  const { HOST } = await import('./types');
  const { chineseCopy } = await import('../constants/copy');
  const { englishCopy } = await import('../i18n/en');
  expect(BUILD_CHANNEL).toBe('dev');
  expect(HOST).toBe('com.knothhe.passpine.dev');
  expect(chineseCopy.brand).toBe('Passpine Dev');
  expect(chineseCopy.connection.openClient).toContain('Passpine Dev');
  expect(englishCopy.connection.openClient).toContain('Passpine Dev');
});

it('compiled release identity keeps the official host and bilingual brand', async () => {
  vi.stubGlobal('__PASSPINE_BUILD_CHANNEL__', 'release');
  vi.resetModules();
  const { HOST } = await import('./types');
  const { chineseCopy } = await import('../constants/copy');
  const { englishCopy } = await import('../i18n/en');
  expect(HOST).toBe('com.knothhe.passpine');
  expect(chineseCopy.brand).toBe('Passpine');
  expect(englishCopy.brand).toBe('Passpine');
});
