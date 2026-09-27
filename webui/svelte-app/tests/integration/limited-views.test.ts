import { describe, expect, it } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
// The real route component: opening a paste link must not burn a view.
import PastePage from '../../src/routes/p/[id]/+page.svelte';
import { apiClient } from '$lib/api/client';
import { getPaste } from '$lib/api/pastes';
import { auth } from '$lib/stores/auth';
import { makeUser } from '../harness/client';
import { makePage, page } from '../shims/app-stores';

describe('limited-view pastes against the real server', () => {
	it('asks before using up a view, and only the confirmed view consumes one', async () => {
		const owner = await makeUser();
		const created = await apiClient.post<{ paste_id: string }>(
			'/paste',
			{
				name: 'one-time.txt',
				language: 'text',
				data: btoa('burn after reading'),
				public: false,
				max_accesses: 2
			},
			owner.token
		);

		auth.login({ token: owner.token });
		page.set({ ...makePage(`/p/${created.paste_id}`), params: { id: created.paste_id } });
		render(PastePage);

		// Opening the page only shows the notice; nothing is consumed yet.
		const view = await screen.findByRole('button', { name: /view paste/i });
		expect(screen.queryByText('one-time.txt')).toBeNull();

		await userEvent.click(view);
		await screen.findByText('one-time.txt');

		// The confirmed view was the first of two: one more read is left,
		// after which the paste is gone.
		const second = await getPaste(created.paste_id, owner.token, true);
		expect(second.access_count).toBe(2);
		await expect(getPaste(created.paste_id, owner.token, true)).rejects.toMatchObject({ status: 404 });
	});

	it('refuses an unconfirmed read without consuming it', async () => {
		const owner = await makeUser();
		const created = await apiClient.post<{ paste_id: string }>(
			'/paste',
			{ name: 'guarded.txt', language: 'text', data: btoa('x'), public: false, max_accesses: 1 },
			owner.token
		);
		await expect(getPaste(created.paste_id, owner.token)).rejects.toMatchObject({ status: 403 });
		await expect(getPaste(created.paste_id, owner.token)).rejects.toMatchObject({ status: 403 });
		const served = await getPaste(created.paste_id, owner.token, true);
		expect(served.name).toBe('guarded.txt');
	});
});
