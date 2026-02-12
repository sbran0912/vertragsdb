import { defineConfig } from 'vite';

export default defineConfig({
  server: {
    proxy: {
      '/api': {
        target: 'http://localhost:8080',
        changeOrigin: true,
      },
    },
  },
  build: {
    outDir: 'dist',
    rollupOptions: {
      input: {
        main: './index.html',
      },
    },
  },
  define: {
    // Für separates Deployment: Backend-URL als Umgebungsvariable
    // 'import.meta.env.VITE_API_URL': JSON.stringify(process.env.VITE_API_URL || '')
  },
});
