/// <reference types='vitest' />
import { defineConfig } from 'vitest/config';
import angular from '@analogjs/vite-plugin-angular';
import { nxViteTsPaths } from '@nx/vite/plugins/nx-tsconfig-paths.plugin';

export default defineConfig(() => ({
  root: __dirname,
  resolve: {
    dedupe: [
      '@angular/core',
      '@angular/common',
      '@angular/platform-browser',
      '@angular/platform-browser-dynamic',
      '@angular/router',
      '@angular/fire',
      'rxjs',
      'zone.js',
    ],
  },
  plugins: [angular({ jit: true, tsconfig: './tsconfig.spec.json' }), nxViteTsPaths()],
  test: {
    name: 'ext-calendarius-contract',
    watch: false,
    globals: true,
    pool: 'forks',
    isolate: true,
    environment: 'jsdom',
    environmentOptions: { jsdom: { url: 'http://localhost/' } },
    include: ['src/**/*.spec.ts'],
    setupFiles: ['src/test-setup.ts'],
    dangerouslyIgnoreUnhandledErrors: true,
    reporters: ['default'],
    server: {
      deps: {
        inline: [
          '@ionic/angular',
          '@ionic/angular/standalone',
          '@angular/fire',
          /@angular\//,
          /@stencil\//,
          /@ionic/,
          /ionicons/,
          /@sneat/,
          /tslib/,
        ],
      },
    },
  },
}));
