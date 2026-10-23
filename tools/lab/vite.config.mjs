import { fileURLToPath } from 'node:url';
import { defineConfig } from 'vite';

// The engine and MediaPipe assets of vitals-web are served from the root of the lab (assetsBaseUrl '/').
export default defineConfig({
  publicDir: fileURLToPath(new URL('../../packages/vitals-web/assets', import.meta.url)),
  build: { outDir: 'dist', emptyOutDir: true, target: 'es2022' },
  worker: { format: 'es' },
});
