// Serves the built lab with the same headers as the demo. Usage: npm run build --workspace tools/lab && npm run serve --workspace tools/lab
import { fileURLToPath } from 'node:url';
import { createServer } from '../../apps/demo/serve.mjs';

const port = Number(process.env.PORT ?? 4301);
createServer(fileURLToPath(new URL('./dist', import.meta.url))).listen(port, '127.0.0.1', () => {
  console.log(`lab on http://127.0.0.1:${String(port)}/`);
});
