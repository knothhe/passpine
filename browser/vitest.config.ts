import { defineConfig } from 'vitest/config';
export default defineConfig({ test: { env: { PASSPINE_BUILD_CHANNEL: 'dev' }, include: ['src/**/*.test.ts'], setupFiles: ['src/test-setup.ts'] } });
