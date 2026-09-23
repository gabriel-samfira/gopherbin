import { describe, expect, it } from 'vitest';
import { render, screen, waitFor, cleanup } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
// Rendering the real route component: everything from the onMount load over
// the API down to the role dropdowns participates in the test.
import TeamPage from '../../src/routes/teams/[name]/+page.svelte';
import { acceptTeamInvite, addTeamMember, createTeam, listTeamMembers } from '$lib/api/teams';
import { login } from '$lib/api/auth';
import { auth } from '$lib/stores/auth';
import { harness, makeUser } from '../harness/client';
import { makePage, page } from '../shims/app-stores';

let seq = 0;
function teamName(): string {
	seq += 1;
	return `roles-team-${Date.now().toString(36)}-${seq}`;
}

async function openTeam(name: string, token: string): Promise<void> {
	cleanup();
	auth.login({ token });
	page.set({ ...makePage(`/teams/${name}`), params: { name } });
	render(TeamPage);
	await screen.findByText(name);
}

describe('team member roles page against the real server', () => {
	it('lets the owner demote a member and persists it server-side', async () => {
		const h = harness();
		const team = teamName();
		await createTeam(team, h.admin.token);
		const member = await makeUser();
		await addTeamMember(team, member.username, h.admin.token, 'member');

		await openTeam(team, h.admin.token);

		// Members table rendered from the server response; the member row is
		// labelled with the "Member" badge.
		await screen.findByText(member.username);
		expect(screen.getAllByText('Member').length).toBeGreaterThan(0);

		// Owner sees the role picker (once in the desktop table, once in the
		// mobile card list).
		const selects = await screen.findAllByRole('combobox', { name: `Role for ${member.username}` });
		expect(selects.length).toBeGreaterThanOrEqual(1);
		await userEvent.selectOptions(selects[0], 'viewer');

		await waitFor(
			async () => {
				const members = await listTeamMembers(team, h.admin.token);
				expect(members.find((m) => m.username === member.username)?.role).toBe('viewer');
			},
			{ timeout: 5000 }
		);

		// Owner can also remove people (the page re-renders while the role
		// change propagates, so wait for the table to settle).
		await waitFor(() => {
			expect(screen.getAllByTitle('Remove from team').length).toBeGreaterThan(0);
		});
	});

	it('hides management affordances from a viewer', async () => {
		const h = harness();
		const team = teamName();
		await createTeam(team, h.admin.token);
		const viewer = await makeUser();
		await addTeamMember(team, viewer.username, h.admin.token, 'viewer');
		await acceptTeamInvite(team, viewer.token);

		await openTeam(team, viewer.token);
		await waitFor(() => expect(screen.getAllByText('Viewer').length).toBeGreaterThan(0));

		expect(screen.queryAllByRole('combobox', { name: `Role for ${viewer.username}` })).toEqual([]);
		expect(screen.queryByRole('combobox', { name: 'Role for harnessadmin' })).toBeNull();
		expect(screen.queryByRole('button', { name: 'Send invite' })).toBeNull();
		expect(screen.queryAllByTitle('Remove from team')).toEqual([]);
	});

	it('hides role pickers from a team-admin but keeps removal of plain members', async () => {
		const h = harness();
		const team = teamName();
		await createTeam(team, h.admin.token);
		const admin = await makeUser();
		const plain = await makeUser();
		await addTeamMember(team, admin.username, h.admin.token, 'admin');
		await addTeamMember(team, plain.username, h.admin.token, 'member');
		await acceptTeamInvite(team, admin.token);
		await acceptTeamInvite(team, plain.token);

		// Role assignment is owner-only; an admin can manage members but not
		// roles, and cannot remove other admins. The invite form's own role
		// picker is not a member "Role for ..." control.
		await openTeam(team, admin.token);
		await screen.findByText(plain.username);

		expect(screen.queryAllByRole('combobox', { name: /^Role for / })).toEqual([]);
		expect(screen.getAllByText('Admin').length).toBeGreaterThan(0);
		const removeButtons = screen.getAllByTitle('Remove from team');
		expect(removeButtons.length).toBeGreaterThanOrEqual(1);

		// ...while the owner sees the full picker (including "promote to
		// admin") on every non-owner row.
		await openTeam(team, h.admin.token);
		await screen.findByText(plain.username);
		const plainSelects = await screen.findAllByRole('combobox', { name: `Role for ${plain.username}` });
		expect(plainSelects[0].querySelector('option[value="admin"]')).not.toBeNull();
		const adminSelects = await screen.findAllByRole('combobox', { name: `Role for ${admin.username}` });
		expect(adminSelects.length).toBeGreaterThanOrEqual(1);
	});
});
