// Static server for the production build that applies the exact CSP of the demo (global constraints).
// Usage: npm run build --workspace apps/demo && node apps/demo/serve.mjs   (PORT env, default 4300)
import { createReadStream, statSync } from 'node:fs';
import http from 'node:http';
import { extname, join, resolve, sep } from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';

export const CSP = [
  "default-src 'self'",
  "connect-src 'self'",
  "script-src 'self' 'wasm-unsafe-eval'",
  "worker-src 'self' blob:",
  "img-src 'self' blob: data:",
  "style-src 'self'",
  "object-src 'none'",
  "base-uri 'none'",
  "frame-ancestors 'none'",
].join('; ');

const HEADERS = {
  'Content-Security-Policy': CSP,
  'X-Content-Type-Options': 'nosniff',
  'Referrer-Policy': 'no-referrer',
  'Permissions-Policy': 'camera=(self), microphone=()',
  'Cache-Control': 'no-cache',
};

const MIME = {
  '.html': 'text/html; charset=utf-8',
  '.js': 'text/javascript; charset=utf-8',
  '.mjs': 'text/javascript; charset=utf-8',
  '.css': 'text/css; charset=utf-8',
  '.json': 'application/json; charset=utf-8',
  '.svg': 'image/svg+xml',
  '.ico': 'image/x-icon',
  '.txt': 'text/plain; charset=utf-8',
  '.wasm': 'application/wasm',
  '.task': 'application/octet-stream',
};

function send(res, status, body) {
  res.writeHead(status, { ...HEADERS, 'Content-Type': 'text/plain; charset=utf-8' });
  res.end(body);
}

// Returns the file to serve, or null for anything outside rootDir, including '..' in raw or percent-encoded form.
function resolveFile(rootDir, rawUrl) {
  const rawPath = rawUrl.split('?')[0] ?? '';
  let path;
  try {
    path = decodeURIComponent(rawPath);
  } catch {
    return null;
  }
  if (!path.startsWith('/') || path.includes('..') || path.includes('\\') || path.includes('\0')) return null;
  const file = resolve(rootDir, `.${path}`);
  if (file !== rootDir && !file.startsWith(rootDir + sep)) return null;
  if (path.endsWith('/') || extname(path) === '') return { file: join(rootDir, 'index.html'), fallback: true };
  return { file, fallback: false };
}

function isFile(file) {
  try {
    return statSync(file).isFile();
  } catch {
    return false;
  }
}

export function createServer(rootDir) {
  const root = resolve(rootDir);
  return http.createServer((req, res) => {
    if (req.method !== 'GET' && req.method !== 'HEAD') {
      send(res, 405, 'Method Not Allowed');
      return;
    }
    const target = resolveFile(root, req.url ?? '/');
    if (target === null || !isFile(target.file)) {
      send(res, 404, 'Not Found');
      return;
    }
    const type = MIME[extname(target.file)] ?? 'application/octet-stream';
    res.writeHead(200, { ...HEADERS, 'Content-Type': type });
    if (req.method === 'HEAD') {
      res.end();
      return;
    }
    createReadStream(target.file)
      .on('error', () => res.destroy())
      .pipe(res);
  });
}

if (process.argv[1] !== undefined && import.meta.url === pathToFileURL(resolve(process.argv[1])).href) {
  const rootDir = fileURLToPath(new URL('./dist/demo/browser/', import.meta.url));
  const port = Number(process.env.PORT ?? 4300);
  createServer(rootDir).listen(port, '127.0.0.1', () => {
    console.log(`demo on http://127.0.0.1:${port}/ (serving ${rootDir})`);
  });
}
