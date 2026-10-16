import tailwindcss from '@tailwindcss/vite';
import { svelte } from '@sveltejs/vite-plugin-svelte';
import { defineConfig } from 'vite';
import { createHash } from 'node:crypto';
import { existsSync, readdirSync, readFileSync, statSync } from 'node:fs';
import { dirname, join, relative, sep } from 'node:path';
import { fileURLToPath } from 'node:url';

// Ports come from the environment so dev and prod never collide:
// dev API 8910 / UI 5190, prod API 8920 / UI 8921 (see package.json scripts).
const uiPort = Number(process.env.UI_PORT ?? 5190);
const apiTarget = `http://127.0.0.1:${process.env.API_PORT ?? 8910}`;

const webDir = dirname(fileURLToPath(import.meta.url));

// uiSourceHash names the UI source this bundle builds from: every file
// under src/ and public/, plus package.json, vite.config.js and
// index.html, in sorted order, each as its slash-separated path, a newline,
// then its raw bytes, sha256 hex cut to 12. The digest is byte-identical to
// hashWebSource in emberloop/update.go, which stamps the same value into
// the Daemon: equal hashes mean the browser runs the build the Daemon
// serves. Never throws: an unreadable tree builds as "dev".
function uiSourceHash() {
  try {
    const rels = [];
    const walk = (dir) => {
      for (const name of readdirSync(dir)) {
        if (name === 'node_modules' || name === 'dist') continue;
        const p = join(dir, name);
        const st = statSync(p);
        if (st.isDirectory()) walk(p);
        else if (st.isFile()) rels.push(relative(webDir, p).split(sep).join('/'));
      }
    };
    for (const top of ['src', 'public']) {
      const root = join(webDir, top);
      if (existsSync(root)) walk(root);
    }
    for (const fixed of ['package.json', 'vite.config.js', 'index.html']) {
      const p = join(webDir, fixed);
      if (existsSync(p) && statSync(p).isFile()) rels.push(fixed);
    }
    if (!rels.length) return 'dev';
    rels.sort();
    const h = createHash('sha256');
    for (const rel of rels) {
      h.update(rel + '\n');
      h.update(readFileSync(join(webDir, ...rel.split('/'))));
    }
    return h.digest('hex').slice(0, 12);
  } catch {
    return 'dev';
  }
}

// Static client-only build: no server rendering, root-absolute asset paths so
// the bundle loads from nested client routes (/s/<id>, /runs/...) too. Point
// the UI at a Daemon with VITE_API_BASE at build time or window.__EL_API_BASE__
// at runtime; same-origin by default.
export default defineConfig({
  plugins: [tailwindcss(), svelte()],
  base: '/',
  ssr: { noExternal: true },
  build: process.env.EL_SSR_HYDRATE_BUILD === '1'
    ? {
        outDir: 'dist/ssr',
        emptyOutDir: false,
        copyPublicDir: false,
        lib: {
          entry: join(webDir, 'src/ssr-hydrate.js'),
          name: 'ELSSRHydrate',
          formats: ['iife'],
          fileName: 'hydrate',
        },
      }
    : {
        outDir: 'dist',
        emptyOutDir: true,
        copyPublicDir: !process.argv.includes('--ssr'),
      },
  define: { __EL_UI_HASH__: JSON.stringify(uiSourceHash()) },
  // Tests mount real components in jsdom, so resolve the client build of Svelte.
  resolve: { conditions: ['browser', 'module', 'import', 'default'] },
  server: { port: uiPort, strictPort: true, proxy: { '/api': apiTarget } },
  preview: { port: uiPort, strictPort: true, proxy: { '/api': apiTarget } },
  test: { environment: 'jsdom', setupFiles: ['./vitest.setup.js'] },
});
