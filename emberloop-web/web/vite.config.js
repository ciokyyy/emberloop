import tailwindcss from '@tailwindcss/vite';
import { svelte } from '@sveltejs/vite-plugin-svelte';
import { defineConfig } from 'vite';

// Static client-only build: no server rendering, relative asset paths so the
// Daemon can serve the bundle from any loopback address. Point the UI at a
// Daemon with VITE_API_BASE at build time or window.__EMBERLOOP_API_BASE__
// at runtime; same-origin by default.
export default defineConfig({
  plugins: [tailwindcss(), svelte()],
  base: './',
  build: { outDir: 'dist', emptyOutDir: true },
  // Tests mount real components in jsdom, so resolve the client build of Svelte.
  resolve: { conditions: ['browser', 'module', 'import', 'default'] },
  server: { port: 5173, proxy: { '/api': 'http://127.0.0.1:8899' } },
  test: { environment: 'jsdom' },
});
