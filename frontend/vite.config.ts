import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

export default defineConfig({
  plugins: [react()],
  server: { port: 5173, proxy: { '/api': 'http://localhost:8080', '/ws': { target: 'ws://localhost:8080', ws: true } } },
  build: {
    outDir: 'dist',
    // Compression-size reporting is useful locally but adds an avoidable pass
    // to every production build on a low-powered VPS.
    reportCompressedSize: false,
    target: 'es2022',
  },
})
