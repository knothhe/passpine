import { defineConfig } from 'wxt';
import { readFileSync } from 'node:fs';
import { buildChannel, assertReleaseBuild } from '../scripts/build-channel.mjs';
import { extensionCommands } from './src/lib/shortcuts';
import { copyHTML } from './plugins/copy-html';

const channel = buildChannel();
if (channel === 'release') assertReleaseBuild();
const identities = JSON.parse(readFileSync(new URL('../config/browser-identities.json', import.meta.url), 'utf8'));
const identity = identities[channel];
const localizedName = channel === 'release' ? '__MSG_extensionName__' : '__MSG_extensionNameDev__';
const localizedDescription = channel === 'release' ? '__MSG_extensionDescription__' : '__MSG_extensionDescriptionDev__';
const commands = channel === 'release' ? extensionCommands : Object.fromEntries(Object.entries(extensionCommands).map(([name, command]) => [name, { description: name === '_execute_action' ? '__MSG_openAppDev__' : command.description }]));

const toolbarIcons = { 16: '/icon16-monochrome.png', 24: '/icon24-monochrome.png', 32: '/icon32-monochrome.png', 48: '/icon48-monochrome.png', 128: '/icon128-monochrome.png' };

export default defineConfig({
  modules: ['@wxt-dev/module-react'],
  hooks: { 'build:manifestGenerated': (_wxt, manifest) => {
    // WXT derives action titles from the popup HTML after merging its metadata.
    // The browser-managed title must retain the channel-specific locale key.
    if (manifest.action) manifest.action.default_title = localizedName;
  } },
  outDir: channel === 'release' ? '.output' : '.output-dev',
  zip: { name: channel === 'release' ? 'passpine' : 'passpine-dev', zipSources: false },
  vite: () => ({ define: { __PASSPINE_BUILD_CHANNEL__: JSON.stringify(channel) }, plugins: [copyHTML()] }),
  manifest: {
    key: identity.key,
    name: localizedName,
    description: localizedDescription,
    default_locale: 'en',
    permissions: ['nativeMessaging', 'activeTab', 'scripting', 'storage', 'clipboardWrite'],
    icons: toolbarIcons,
    action: { default_icon: toolbarIcons, default_title: localizedName },
    commands,
    browser_specific_settings: { gecko: { id: identity.firefoxID, strict_min_version: '128.0', data_collection_permissions: { required: ['none'] } } },
    // Radix uses inline custom properties for component sizing and positioning.
    content_security_policy: { extension_pages: "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; object-src 'none'; connect-src 'none'" },
  },
});
