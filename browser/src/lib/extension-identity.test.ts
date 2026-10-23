import { afterEach, expect, it, vi } from 'vitest';

afterEach(() => { vi.unstubAllGlobals(); vi.unstubAllEnvs(); vi.resetModules(); });

it.each(['', 'dev', 'release'])('keeps unified bilingual branding with desktop channel %j', async channel => {
  vi.stubEnv('PASSPINE_BUILD_CHANNEL', channel);
  vi.stubGlobal('__PASSPINE_BUILD_CHANNEL__', 'dev');
  vi.resetModules();
  const { chineseCopy } = await import('../constants/copy');
  const { englishCopy } = await import('../i18n/en');
  expect(chineseCopy.brand).toBe('Passpine');
  expect(englishCopy.brand).toBe('Passpine');
});
