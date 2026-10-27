import { defineConfig } from 'vite';
import { svelte } from '@sveltejs/vite-plugin-svelte';
import { writeFileSync } from 'node:fs';
import { resolve } from 'node:path';

const outDir = resolve(import.meta.dirname, '../backend/internal/web/dist');

export default defineConfig({
  plugins: [
    svelte(),
    {
      // The build empties outDir; go:embed needs the directory to be non-empty
      // even before the first frontend build, so restore the placeholder.
      name: 'keep-embed-placeholder',
      closeBundle() {
        writeFileSync(resolve(outDir, '.gitkeep'), '');
      },
    },
  ],
  build: {
    outDir,
    emptyOutDir: true,
    // Baseline assumption for older Android tablets, see docs/adr/006.
    target: 'chrome80',
    cssTarget: 'chrome80',
  },
  server: {
    proxy: {
      '/ws': { target: 'ws://localhost:8080', ws: true },
    },
  },
});
