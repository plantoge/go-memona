import { defineConfig } from 'vite';
import { svelte } from '@sveltejs/vite-plugin-svelte';

export default defineConfig({
  plugins: [svelte()],
  build: {
    outDir: '../internal/webui/dist',
    // Keep existing generated releases; no automatic deletion of project files.
    emptyOutDir: false,
    sourcemap: false,
  },
  server: {
    host: '127.0.0.1',
    port: 5173,
    strictPort: true,
    proxy: {
      '/api': {
        target: 'http://127.0.0.1:8765',
        changeOrigin: true,
        // Only this loopback dev proxy rewrites Origin to its fixed upstream.
        // Go still enforces Host/Origin/CSRF. Do not expose Vite publicly.
        configure(proxy) {
          proxy.on('proxyReq', (proxyReq, req) => {
            if (req.headers.origin === 'http://127.0.0.1:5173') proxyReq.setHeader('Origin', 'http://127.0.0.1:8765');
          });
        },
      },
    },
  },
});
