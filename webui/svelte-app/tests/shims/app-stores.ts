import { writable } from 'svelte/store';

export function makePage(pathname = '/') {
	return {
		url: new URL(pathname, 'http://localhost/'),
		route: { id: null },
		params: {} as Record<string, string>,
		data: {},
		form: null,
		state: {},
		error: null,
		status: 200
	};
}

export const page = writable(makePage());
