import { describe, expect, it, vi } from 'vitest';
import { login, logout as apiLogout } from '$lib/api/auth';
import { getMe } from '$lib/api/users';
import { auth } from '$lib/stores/auth';
import { harness, makeUser } from '../harness/client';

function current<T>(store: { subscribe: (fn: (v: T) => void) => () => void }): T {
	let value!: T;
	const unsubscribe = store.subscribe((v) => (value = v));
	unsubscribe();
	return value;
}

describe('authentication against the real server', () => {
	it('logs in with a real account and exposes the session everywhere', async () => {
		const user = await makeUser();
		const session = await login({ username: user.username, password: user.password });

		auth.login(session);
		const state = current(auth);

		expect(state.isAuthenticated).toBe(true);
		expect(state.token).toBe(session.token);
		expect(state.isAdmin).toBe(false);
		expect(localStorage.getItem('authToken')).toBe(session.token);
		expect(Number(localStorage.getItem('username'))).toBeGreaterThan(0);

		// The token is not a fixture: the server accepts it on a later call.
		const me = await getMe(session.token);
		expect(me.username).toBe(user.username);
		expect(me.is_admin).toBe(false);
	});

	it('fails login with a wrong password without touching the session', async () => {
		const user = await makeUser();
		let caught: unknown;
		try {
			await login({ username: user.username, password: 'definitely-not-the-password' });
		} catch (err) {
			caught = err;
		}
		expect(caught).toBeTruthy();
		expect((caught as { error?: string }).error).toBeTruthy();
		expect(current(auth).isAuthenticated).toBe(false);
		expect(localStorage.getItem('authToken')).toBeNull();
	});

	it('rehydrates a session from localStorage on a fresh boot', async () => {
		const user = await makeUser();
		const session = await login({ username: user.username, password: user.password });
		localStorage.setItem('authToken', session.token);

		// Simulate a full page reload by re-initializing the store module.
		vi.resetModules();
		const { auth: rehydrated } = await import('$lib/stores/auth');
		const state = current(rehydrated);

		expect(state.isAuthenticated).toBe(true);
		expect(state.token).toBe(session.token);
		expect(state.fullName).toContain('Testuser');
	});

	it('drops a corrupt token on a fresh boot', async () => {
		localStorage.setItem('authToken', 'not.a.real.jwt');

		vi.resetModules();
		const { auth: rehydrated } = await import('$lib/stores/auth');

		expect(current(rehydrated).isAuthenticated).toBe(false);
		expect(localStorage.getItem('authToken')).toBeNull();
	});

	it('logout invalidates the session server-side too', async () => {
		const user = await makeUser();
		const session = await login({ username: user.username, password: user.password });
		auth.login(session);
		// The full logout the UI performs: server-side blacklist, then store.
		await apiLogout(session.token);
		auth.logout();

		expect(current(auth).isAuthenticated).toBe(false);
		expect(localStorage.getItem('authToken')).toBeNull();
		// The token is now blacklisted and rejected by the server.
		await expect(getMe(session.token)).rejects.toBeTruthy();
		// Sanity: the admin token from the harness still works.
		expect((await getMe(harness().admin.token)).username).toBe('harnessadmin');
	});
});
