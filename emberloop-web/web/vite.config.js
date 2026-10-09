import tailwindcss from '@tailwindcss/vite';
import { svelte } from '@sveltejs/vite-plugin-svelte';
import { defineConfig } from 'vite';

// Ports come from the environment so dev and prod never collide:
// dev API 8910 / UI 5190, prod API 8920 / UI 8921 (see package.json scripts).
const uiPort = Number(process.env.UI_PORT ?? 5190);
const apiTarget = `http://127.0.0.1:${process.env.API_PORT ?? 8910}`;

// Static client-only build: no server rendering, relative asset paths so the
// Daemon can serve the bundle from any loopback address. Point the UI at a
// Daemon with VITE_API_BASE at build time or window.__EL_API_BASE__
// at runtime; same-origin by default.
export default defineConfig({
  plugins: [tailwindcss(), svelte()],
  base: './',
  build: { outDir: 'dist', emptyOutDir: true },
  // Tests mount real components in jsdom, so resolve the client build of Svelte.
  resolve: { conditions: ['browser', 'module', 'import', 'default'] },
  server: { port: uiPort, strictPort: true, proxy: { '/api': apiTarget } },
  preview: { port: uiPort, strictPort: true, proxy: { '/api': apiTarget } },
  test: { environment: 'jsdom' },
});
