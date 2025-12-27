import { writable } from 'svelte/store';
import { browser } from '$app/environment';
import { jwtDecode } from 'jwt-decode';
import type { LoginResponse, JWTPayload } from '$lib/types/api';

interface AuthState {
	token: string | null;
	isAdmin: boolean;
	username: string | null;
	fullName: string | null;
	isAuthenticated: boolean;
}

function createAuthStore() {
	const getInitialState = (): AuthState => {
		if (!browser) {
			return {
				token: null,
				isAdmin: false,
				username: null,
				fullName: null,
				isAuthenticated: false
			};
		}

		const token = localStorage.getItem('authToken');

		// If we have a token, decode it to get fresh user info
		if (token) {
			try {
				const payload = jwtDecode<JWTPayload>(token);

				// Update localStorage with decoded values
				localStorage.setItem('isAdmin', String(payload.is_admin));
				localStorage.setItem('username', String(payload.user));
				localStorage.setItem('fullName', payload.full_name);

				return {
					token,
					isAdmin: payload.is_admin,
					username: String(payload.user),
					fullName: payload.full_name,
					isAuthenticated: true
				};
			} catch (err) {
				// Token is invalid, clear everything
				localStorage.removeItem('authToken');
				localStorage.removeItem('isAdmin');
				localStorage.removeItem('username');
				localStorage.removeItem('fullName');
			}
		}

		return {
			token: null,
			isAdmin: false,
			username: null,
			fullName: null,
			isAuthenticated: false
		};
	};

	const { subscribe, set, update } = writable<AuthState>(getInitialState());

	return {
		subscribe,
		login: (data: LoginResponse) => {
			// Decode JWT to get user information
			const payload = jwtDecode<JWTPayload>(data.token);

			if (browser) {
				localStorage.setItem('authToken', data.token);
				localStorage.setItem('isAdmin', String(payload.is_admin));
				localStorage.setItem('username', String(payload.user));
				localStorage.setItem('fullName', payload.full_name);
			}

			set({
				token: data.token,
				isAdmin: payload.is_admin,
				username: String(payload.user),
				fullName: payload.full_name,
				isAuthenticated: true
			});
		},
		logout: () => {
			if (browser) {
				localStorage.removeItem('authToken');
				localStorage.removeItem('isAdmin');
				localStorage.removeItem('username');
				localStorage.removeItem('fullName');
			}

			set({
				token: null,
				isAdmin: false,
				username: null,
				fullName: null,
				isAuthenticated: false
			});
		},
		checkAuth: () => {
			update((state) => state);
		}
	};
}

export const auth = createAuthStore();
