import { mkdtempSync, mkdirSync, rmSync, writeFileSync } from 'node:fs';
import { request } from 'node:http';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { CSP, createServer } from './serve.mjs';

const EXACT_CSP =
  "default-src 'self'; connect-src 'self'; script-src 'self' 'wasm-unsafe-eval'; worker-src 'self' blob:; " +
  "img-src 'self' blob: data:; style-src 'self'; object-src 'none'; base-uri 'none'; frame-ancestors 'none'";

let root;
let server;
let port;

// Raw request so that '..' and '%2e%2e' reach the server unnormalised.
function get(path, method = 'GET') {
  return new Promise((resolve, reject) => {
    const req = request({ host: '127.0.0.1', port, path, method }, (res) => {
      const chunks = [];
      res.on('data', (c) => chunks.push(c));
      res.on('end', () => {
        resolve({ status: res.statusCode, headers: res.headers, body: Buffer.concat(chunks).toString() });
      });
    });
    req.on('error', reject);
    req.end();
  });
}

beforeAll(async () => {
  const base = mkdtempSync(join(tmpdir(), 'serve-test-'));
  root = join(base, 'browser');
  mkdirSync(join(root, 'assets', 'vitals', 'mediapipe'), { recursive: true });
  writeFileSync(join(base, 'package.json'), '{"secret":true}');
  writeFileSync(join(root, 'index.html'), '<!doctype html><title>index</title>');
  writeFileSync(join(root, 'main.js'), 'export {};');
  writeFileSync(join(root, 'chunk.mjs'), 'export {};');
  writeFileSync(join(root, 'styles.css'), 'body{}');
  writeFileSync(join(root, 'data.json'), '{}');
  writeFileSync(join(root, 'icon.svg'), '<svg/>');
  writeFileSync(join(root, 'assets', 'vitals', 'engine.wasm'), Buffer.from([0, 97, 115, 109]));
  writeFileSync(join(root, 'assets', 'vitals', 'mediapipe', 'face_landmarker.task'), 'task');
  server = createServer(root);
  await new Promise((resolve) => server.listen(0, '127.0.0.1', resolve));
  port = server.address().port;
});

afterAll(async () => {
  await new Promise((resolve) => server.close(resolve));
  rmSync(join(root, '..'), { recursive: true, force: true });
});

describe('serve.mjs', () => {
  it('exports the exact CSP from the global constraints', () => {
    expect(CSP).toBe(EXACT_CSP);
  });

  it('sends the exact CSP and the hardening headers', async () => {
    const res = await get('/');

    expect(res.status).toBe(200);
    expect(res.headers['content-security-policy']).toBe(EXACT_CSP);
    expect(res.headers['x-content-type-options']).toBe('nosniff');
    expect(res.headers['referrer-policy']).toBe('no-referrer');
    expect(res.headers['permissions-policy']).toBe('camera=(self), microphone=()');
    expect(res.headers['content-type']).toBe('text/html; charset=utf-8');
    expect(res.body).toContain('<title>index</title>');
  });

  it('sends the hardening headers on errors too', async () => {
    const res = await get('/missing.js');

    expect(res.status).toBe(404);
    expect(res.headers['content-security-policy']).toBe(EXACT_CSP);
    expect(res.headers['x-content-type-options']).toBe('nosniff');
  });

  it.each([
    ['/main.js', 'text/javascript; charset=utf-8'],
    ['/chunk.mjs', 'text/javascript; charset=utf-8'],
    ['/styles.css', 'text/css; charset=utf-8'],
    ['/data.json', 'application/json; charset=utf-8'],
    ['/icon.svg', 'image/svg+xml'],
    ['/assets/vitals/engine.wasm', 'application/wasm'],
    ['/assets/vitals/mediapipe/face_landmarker.task', 'application/octet-stream'],
  ])('serves %s as %s', async (path, type) => {
    const res = await get(path);

    expect(res.status).toBe(200);
    expect(res.headers['content-type']).toBe(type);
  });

  it('falls back to index.html only for paths without an extension', async () => {
    expect((await get('/exercise')).body).toContain('<title>index</title>');
    expect((await get('/exercise?mock=1')).body).toContain('<title>index</title>');
    expect((await get('/nope.wasm')).status).toBe(404);
  });

  it.each([
    '/../package.json',
    '/assets/../../package.json',
    '/%2e%2e/package.json',
    '/%2E%2E/package.json',
    '/assets/%2e%2e%2f%2e%2e%2fpackage.json',
    '/..%2fpackage.json',
    '/%252e%252e/package.json',
    '/..\\package.json',
    '/assets\\..\\..\\package.json',
    '/%5c..%5cpackage.json',
    '/main.js%00.png',
    '/%00/../package.json',
    'http://127.0.0.1/main.js',
    'http://127.0.0.1/../package.json',
    'http://evil.example/package.json',
  ])('rejects traversal %s with 404', async (path) => {
    const res = await get(path);

    expect(res.status).toBe(404);
    expect(res.body).not.toContain('secret');
  });

  it('keeps a double-slash path inside the root: it is only the extensionless fallback', async () => {
    const res = await get('//etc/passwd');

    expect(res.status).toBe(200);
    expect(res.body).toContain('<title>index</title>');
    expect(res.body).not.toContain('root:');
    expect((await get('//etc/passwd.txt')).status).toBe(404);
  });

  it('rejects methods other than GET and HEAD', async () => {
    expect((await get('/', 'POST')).status).toBe(405);
    expect((await get('/', 'HEAD')).status).toBe(200);
  });
});
