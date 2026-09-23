// Worker-side access to the harness. Uses the app's own api client so the
// helper traffic goes through the same code path as the UI.

import { readFileSync } from 'node:fs';
import { createUser } from '$lib/api/users';
import { login } from '$lib/api/auth';
import { HARNESS_FILE, USER_PASSWORD, type Harness } from './server';

let cached: Harness | undefined;

export function harness(): Harness {
	if (!cached) {
		cached = JSON.parse(readFileSync(HARNESS_FILE, 'utf8')) as Harness;
	}
	return cached;
}

let seq = 0;

function unique(prefix: string): string {
	seq += 1;
	return `${prefix}${Date.now().toString(36).slice(-5)}${seq}`;
}

export interface TestUser {
	id: string;
	username: string;
	email: string;
	password: string;
	token: string;
}

/** Creates a real user through the admin API and logs it in for real. */
export async function makeUser(opts: { admin?: boolean; prefix?: string } = {}): Promise<TestUser> {
	const username = unique(opts.prefix ?? 'user');
	const email = `${username}@gopherbin.test`;
	const created = await createUser(
		{
			username,
			email,
			full_name: `${username} Testuser`,
			password: USER_PASSWORD,
			is_admin: !!opts.admin,
			enabled: true
		},
		harness().admin.token
	);
	const session = await login({ username, password: USER_PASSWORD });
	return { id: created.id, username, email, password: USER_PASSWORD, token: session.token };
}

export async function loginToken(username: string): Promise<string> {
	return (await login({ username, password: USER_PASSWORD })).token;
}
