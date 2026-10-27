import { defineConfig } from 'wxt';
import { readFileSync } from 'node:fs';
import { extensionCommands } from './src/lib/shortcuts';
import { copyHTML } from './plugins/copy-html';

const identities = JSON.parse(readFileSync(new URL('../config/browser-identities.json', import.meta.url), 'utf8'));
const identity = identities.release;
const localizedName = '__MSG_extensionName__';
const localizedDescription = '__MSG_extensionDescription__';

const toolbarIcons = { 16: '/icon16-monochrome.png', 24: '/icon24-monochrome.png', 32: '/icon32-monochrome.png', 48: '/icon48-monochrome.png', 128: '/icon128-monochrome.png' };

export default defineConfig({
  modules: ['@wxt-dev/module-react'],
  hooks: { 'build:manifestGenerated': (_wxt, manifest) => {
    // WXT derives action titles from the popup HTML after merging its metadata.
    // The browser-managed title must retain its locale key.
    if (manifest.action) manifest.action.default_title = localizedName;
  } },
  outDir: '.output',
  zip: { name: 'passpine', zipSources: false },
  vite: () => ({ plugins: [copyHTML()] }),
  manifest: ({ browser }) => ({
    key: identity.key,
    name: localizedName,
    description: localizedDescription,
    default_locale: 'en',
    permissions: ['nativeMessaging', 'activeTab', 'scripting', 'storage', 'clipboardWrite'],
    optional_host_permissions: ['http://*/*', 'https://*/*'],
    icons: toolbarIcons,
    action: { default_icon: toolbarIcons, default_title: localizedName },
    commands: extensionCommands(browser === 'firefox'),
    browser_specific_settings: { gecko: { id: identity.firefoxID, strict_min_version: '128.0', data_collection_permissions: { required: ['none'] } } },
    // Radix uses inline custom properties for component sizing and positioning.
    content_security_policy: { extension_pages: "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; object-src 'none'; connect-src 'none'" },
  }),
});
