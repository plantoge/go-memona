import { defineConfig } from 'vite';
import { svelte } from '@sveltejs/vite-plugin-svelte';
import { fileURLToPath } from 'node:url';

export default defineConfig({
  plugins: [
    svelte(),
    {
      name: 'memona-dev-routes',
      configureServer(server) {
        server.middlewares.use((req, res, next) => {
          const pathname = new URL(req.url ?? '/', 'http://localhost').pathname;
          if (pathname === '/admin' || pathname === '/admin/') {
            req.url = '/admin.html';
          } else if (pathname === '/user' || pathname === '/user/' || pathname === '/user.html') {
            res.statusCode = 404;
            res.end('404 page not found\n');
            return;
          }
          next();
        });
      },
    },
  ],
  build: {
    outDir: '../internal/webui/dist',
    // Keep existing generated releases; no automatic deletion of project files.
    emptyOutDir: false,
    sourcemap: false,
    rollupOptions: {
      input: {
        index: fileURLToPath(new URL('./index.html', import.meta.url)),
        admin: fileURLToPath(new URL('./admin.html', import.meta.url)),
      },
    },
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
