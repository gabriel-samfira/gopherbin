import { describe, expect, it } from 'vitest';
import { render, screen, waitFor } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import UserSearchInput from '$lib/components/ui/UserSearchInput.svelte';
import { addTeamMember, createTeam } from '$lib/api/teams';
import { harness, makeUser } from '../harness/client';

describe('user type-ahead against the real server', () => {
	it('finds a real user, lets the caller pick it, and clears after choosing', async () => {
		const h = harness();
		const target = await makeUser({ prefix: 'typeahead' });

		render(UserSearchInput, { props: { token: h.admin.token } });
		const input = screen.getByPlaceholderText('Username or email');

		// Below the minimum length nothing may be requested or shown.
		await userEvent.type(input, target.username.slice(0, 1));
		await new Promise((r) => setTimeout(r, 400));
		expect(screen.queryByRole('listbox')).toBeNull();

		await userEvent.type(input, target.username.slice(1));

		const option = await screen.findByRole('option', { name: new RegExp(target.username) }, { timeout: 5000 });
		await userEvent.click(option);

		await waitFor(() => {
			expect((input as HTMLInputElement).value).toBe(target.username);
		});
		expect(screen.queryByRole('listbox')).toBeNull();
	});

	it('excludes members of the target team from the results', async () => {
		const h = harness();
		const member = await makeUser({ prefix: 'teammate' });
		const team = `search-team-${Date.now().toString(36)}`;
		await createTeam(team, h.admin.token);
		await addTeamMember(team, member.username, h.admin.token);

		render(UserSearchInput, { props: { token: h.admin.token, team } });
		const input = screen.getByPlaceholderText('Username or email');
		await userEvent.type(input, member.username);

		await new Promise((r) => setTimeout(r, 800));
		expect(screen.queryByRole('option', { name: new RegExp(member.username) })).toBeNull();
	});

	it('shows nothing for a query without matches', async () => {
		const h = harness();
		render(UserSearchInput, { props: { token: h.admin.token } });
		const input = screen.getByPlaceholderText('Username or email');
		await userEvent.type(input, 'zz-nobody-here-zz');

		await new Promise((r) => setTimeout(r, 800));
		expect(screen.queryByRole('listbox')).toBeNull();
	});
});
