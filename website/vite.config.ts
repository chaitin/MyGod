import path from 'node:path';
import { fileURLToPath } from 'node:url';
import react from '@vitejs/plugin-react';
import { defineConfig } from 'vite';

const projectRoot = path.dirname(fileURLToPath(import.meta.url));

export default defineConfig({
  base: process.env.VITE_BASE_PATH || '/',
  plugins: [react()],
  publicDir: path.resolve(projectRoot, '../client-web/public'),
  resolve: {
    alias: {
      '@': path.resolve(projectRoot, 'src'),
    },
  },
});
