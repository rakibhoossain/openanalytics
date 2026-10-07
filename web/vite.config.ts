import tailwindcss from '@tailwindcss/vite';
import { tanstackStart } from '@tanstack/react-start/plugin/vite';
import viteReact from '@vitejs/plugin-react';
import { defineConfig } from 'vite';
import viteTsConfigPaths from 'vite-tsconfig-paths';

export default defineConfig({
  plugins: [
    viteTsConfigPaths({
      projects: ['./tsconfig.json'],
    }),
    tailwindcss(),
    tanstackStart(),
    viteReact(),
  ],
  server: {
    port: 3000,
    proxy: {
      '/trpc': {
        target: 'http://localhost:8081',
        changeOrigin: true,
        headers: {
          'X-AUTH-KEY': process.env.ANALYTICS_AUTH_KEY || 'aicart_analytics_internal_secret_key_8503c2a0',
        },
      },
      '/api': {
        target: 'http://localhost:8081',
        changeOrigin: true,
        headers: {
          'X-AUTH-KEY': process.env.ANALYTICS_AUTH_KEY || 'aicart_analytics_internal_secret_key_8503c2a0',
        },
      },
      '/misc': {
        target: 'http://localhost:8081',
        changeOrigin: true,
        headers: {
          'X-AUTH-KEY': process.env.ANALYTICS_AUTH_KEY || 'aicart_analytics_internal_secret_key_8503c2a0',
        },
      },
      '/live': {
        target: 'http://localhost:8081',
        ws: true,
        changeOrigin: true,
      },
    },
  },
  ssr: {
    noExternal: [/@visx\//],
    external: ['lowlight'],
  },
});
