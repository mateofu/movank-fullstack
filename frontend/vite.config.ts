import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vite';

const proxy = { '/v1': { target: process.env.API_URL ?? 'http://127.0.0.1:8080' } };

export default defineConfig({
  plugins: [sveltekit()],
  server: { proxy },
  preview: { proxy }
});
