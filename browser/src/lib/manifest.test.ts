import { expect, it, vi } from 'vitest';
import { readFileSync } from 'node:fs';
import config from '../../wxt.config';
import storeConfig from '../../wxt.store.config';

vi.mock('wxt', () => ({ defineConfig: (config: unknown) => config }));

const identity = JSON.parse(readFileSync(new URL('../../../config/browser-identities.json', import.meta.url), 'utf8')).release;

async function manifest(source: typeof config, browser: 'chrome' | 'firefox', mode: string) {
  return typeof source.manifest === 'function'
    ? source.manifest({ browser, mode, command: mode === 'development' ? 'serve' : 'build', manifestVersion: 3 })
    : source.manifest;
}

it('omits the key field from Chrome Web Store builds', async () => {
  expect(await manifest(storeConfig, 'chrome', 'production')).not.toHaveProperty('key');
  expect(storeConfig.outDirTemplate).toBe('{{browser}}-mv{{manifestVersion}}-store');
});

it.each(['development', 'production'])('preserves the fixed ID for unpacked Chrome %s builds', async mode => {
  await manifest(storeConfig, 'chrome', 'production');
  expect((await manifest(config, 'chrome', mode))?.key).toBe(identity.key);
});

it('retains the Firefox identity when packaging', async () => {
  const result = await manifest(config, 'firefox', 'production');
  expect(result?.key).toBe(identity.key);
  expect(result?.browser_specific_settings?.gecko?.id).toBe(identity.firefoxID);
});
