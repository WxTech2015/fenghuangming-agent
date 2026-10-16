import { defineConfig } from 'vitest/config';
import vue from '@vitejs/plugin-vue';
export default defineConfig({ plugins: [vue()], base: './', build: { target: 'es2018', outDir: '../build/webroot', emptyOutDir: true }, test: { include: ['src/**/*.test.ts'] } });
