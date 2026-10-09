import path from 'node:path';
import type { E2EConfig } from 'e2e';
import { web } from '@e2e-dev/web';

// The suite runs the real Daemon (Go) with the fake backend, serving the
// embedded UI from ../dist: run `npm run build` in .. first. No model is used.
export default {
  targets: [
    {
      engine: web(),
      app: {
        url: 'http://127.0.0.1:0',
        command: {
          executable: path.resolve(import.meta.dirname, 'serve.sh'),
          args: ['{port}'],
          env: { PATH: process.env.PATH ?? '', GOCACHE: process.env.GOCACHE ?? '' },
          startupTimeout: 180_000,
        },
      },
    },
  ],
  workers: 1,
} satisfies E2EConfig;
