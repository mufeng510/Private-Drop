import { defineConfig } from 'vitest/config';
import react from '@vitejs/plugin-react';
import tailwindcss from '@tailwindcss/vite';
import { VitePWA } from 'vite-plugin-pwa';

// The Go backend embeds/serves web/dist and MUST fall back to index.html for
// non-API routes so that BrowserRouter deep links and refreshes work.
export default defineConfig({
  plugins: [
    react(),
    tailwindcss(),
    VitePWA({
      registerType: 'autoUpdate',
      // A custom service worker is required to read the multipart body of a
      // Web Share Target POST — a plain SPA cannot read a cross-navigation POST.
      strategies: 'injectManifest',
      srcDir: 'src',
      filename: 'sw.ts',
      injectManifest: {
        globPatterns: ['**/*.{js,css,html,svg,png,woff2}'],
        maximumFileSizeToCacheInBytes: 5 * 1024 * 1024,
      },
      includeAssets: ['icon.svg', 'apple-touch-icon.png'],
      manifest: {
        name: 'Private Drop',
        short_name: 'Private Drop',
        description: 'Personal NAS message & file drop',
        lang: 'zh-CN',
        start_url: '/',
        scope: '/',
        display: 'standalone',
        background_color: '#f6f7f9',
        theme_color: '#2563eb',
        orientation: 'any',
        icons: [
          { src: 'icon.svg', sizes: 'any', type: 'image/svg+xml', purpose: 'any' },
          { src: 'icon.svg', sizes: 'any', type: 'image/svg+xml', purpose: 'maskable' },
          { src: 'icon-192.png', sizes: '192x192', type: 'image/png', purpose: 'any' },
          { src: 'icon-512.png', sizes: '512x512', type: 'image/png', purpose: 'maskable' },
        ],
        // All modern share targets POST multipart/form-data: text can be shared
        // with any browser that supports the API, files require a service
        // worker (see src/sw.ts). iOS Safari has no share target — use copy /
        // paste or the in-app picker there.
        share_target: {
          action: '/?share-target=1',
          method: 'POST',
          enctype: 'multipart/form-data',
          params: {
            title: 'title',
            text: 'text',
            url: 'url',
            files: [
              { name: 'files', accept: ['image/*', 'video/*', 'audio/*', 'application/pdf', '.txt', '.md'] },
            ],
          },
        },
      },
      devOptions: { enabled: false },
    }),
  ],
  build: {
    target: 'es2022',
    outDir: 'dist',
    emptyOutDir: true,
  },
  test: {
    environment: 'node',
    globals: true,
    include: ['src/**/*.test.ts'],
  },
});
