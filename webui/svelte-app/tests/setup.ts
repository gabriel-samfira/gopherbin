import { afterEach, beforeEach } from 'vitest';
import { cleanup } from '@testing-library/svelte';
import { auth } from '$lib/stores/auth';
import { harness } from './harness/client';

// The app client uses root-relative /api/v1 URLs; resolve them against the
// harness server instead of the jsdom origin. Requests otherwise go out
// unmodified over real HTTP.
const origin = new URL(harness().base).origin;
const realFetch = globalThis.fetch.bind(globalThis);
globalThis.fetch = ((input: RequestInfo | URL, init?: RequestInit) => {
	if (typeof input === 'string' && input.startsWith('/')) {
		return realFetch(new URL(input, origin).href, init);
	}
	return realFetch(input as RequestInfo, init);
}) as typeof fetch;

beforeEach(() => {
	localStorage.clear();
	auth.logout();
});

afterEach(() => {
	cleanup();
});
