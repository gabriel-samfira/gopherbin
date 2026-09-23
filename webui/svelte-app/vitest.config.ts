import path from 'node:path';
import { defineConfig } from 'vitest/config';
import { svelte } from '@sveltejs/vite-plugin-svelte';

const r = (p: string) => path.resolve(import.meta.dirname, p);

export default defineConfig({
	plugins: [svelte()],
	resolve: {
		// Force the browser build of Svelte (and its deps) inside jsdom;
		// otherwise the SSR export condition resolves svelte/index-server
		// and mount() becomes unavailable.
		conditions: ['browser'],
		alias: [
			{ find: '$app/environment', replacement: r('tests/shims/app-environment.ts') },
			{ find: '$app/navigation', replacement: r('tests/shims/app-navigation.ts') },
			{ find: '$app/stores', replacement: r('tests/shims/app-stores.ts') },
			{ find: '$lib', replacement: r('src/lib') }
		]
	},
	test: {
		include: ['tests/**/*.test.ts', 'src/**/*.test.ts'],
		globalSetup: ['tests/harness/server.ts'],
		setupFiles: ['tests/setup.ts'],
		environment: 'jsdom',
		environmentOptions: {
			jsdom: { url: 'http://localhost/' }
		},
		// Tests share one live server with a SQLite database; run files serially
		// to keep the harness logs and data deterministic.
		fileParallelism: false,
		testTimeout: 30000,
		hookTimeout: 300000
	}
});
