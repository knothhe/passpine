import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import { copyHTML } from './plugins/copy-html';

export default defineConfig({
  root: 'preview', publicDir: '../public', plugins: [react(), copyHTML()],
  server: { host: '127.0.0.1', port: 5174, strictPort: true, fs: { allow: ['..'] } },
});
