import { defineConfig } from 'vite'
import { writeFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'

// Built assets land inside the Go package that embeds them. `go:embed all:dist`
// needs the directory to exist and to match at least one file, so the marker is
// restored after Vite's `emptyOutDir` wipes it.
const distDir = fileURLToPath(new URL('../internal/web/dist', import.meta.url))
const marker = new URL('../internal/web/dist/.gitkeep', import.meta.url)

export default defineConfig({
  base: './',
  build: {
    outDir: distDir,
    emptyOutDir: true,
    target: 'es2022',
    assetsDir: 'assets',
    sourcemap: false,
    chunkSizeWarningLimit: 1024,
  },
  server: {
    port: 5173,
    strictPort: true,
  },
  plugins: [
    {
      name: 'beacon:keep-embed-dir',
      closeBundle() {
        writeFileSync(marker, '')
      },
    },
  ],
})