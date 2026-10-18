import { defineConfig } from 'wxt';
import { extensionCommands } from './src/lib/shortcuts';
import { copyHTML } from './plugins/copy-html';

const toolbarIcons = { 16: '/icon16-monochrome.png', 32: '/icon32-monochrome.png', 48: '/icon48-monochrome.png', 128: '/icon128-monochrome.png' };

export default defineConfig({
  modules: ['@wxt-dev/module-react'],
  vite: () => ({ plugins: [copyHTML()] }),
  manifest: {
    key: 'MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEA267SX2CYUamF//05ArsbaN65lyhD3UHhef1CQmPsY1XiZRn/oYJE0919Feo8Nb55oR4Eb0TBjVSWTpq7li9hMe3jNSeiesbHqFSOkG6ezoBuLWQ3QFtOU5k5rlw572Oa0i6/YQJ2TmrFkojUkIJ1FzI97cmS+XUK8+7gSdshKIlz0HD3gxDlJoEQSNpGZg5EnrUUuSdwIhUDBtwWMojpA/U0S5+lOwOg2gogmQfeKhlIPqzBkV1QAv3TDDpYeJFXPDWc1guTeeiPV/FaWxo4BFpU3md6jQegPODMorG+V37fnS8EX8VUUZCylnZzHjstYms6wZ8oTgP1NNzeCqecSwIDAQAB',
    name: '__MSG_extensionName__',
    description: '__MSG_extensionDescription__',
    default_locale: 'en',
    permissions: ['nativeMessaging', 'activeTab', 'scripting', 'storage', 'clipboardWrite'],
    icons: toolbarIcons,
    action: { default_icon: toolbarIcons, default_title: '__MSG_extensionName__' },
    commands: extensionCommands,
    browser_specific_settings: { gecko: { id: 'passpine@knothhe.com', strict_min_version: '128.0', data_collection_permissions: { required: ['none'] } } },
    // Radix uses inline custom properties for component sizing and positioning.
    content_security_policy: { extension_pages: "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; object-src 'none'; connect-src 'none'" },
  },
});
