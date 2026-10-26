import { defineConfig, type UserManifest } from 'wxt';
import config from './wxt.config';

// Keep the normal production mode and leave unpacked builds untouched.
export default defineConfig({
  ...config,
  outDirTemplate: '{{browser}}-mv{{manifestVersion}}-store',
  manifest: async env => {
    const manifest: UserManifest = { ...await (typeof config.manifest === 'function' ? config.manifest(env) : config.manifest) };
    // Chrome Web Store assigns the ID; key is only for stable unpacked IDs.
    if (env.browser === 'chrome') delete manifest.key;
    return manifest;
  },
});
