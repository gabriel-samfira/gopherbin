import { describe, expect, it } from 'vitest';
import { render, screen, waitFor } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import InviteBell from '$lib/components/layout/InviteBell.svelte';
import { acceptTeamInvite, addTeamMember, createTeam, declineTeamInvite, getTeam, listTeamMembers } from '$lib/api/teams';
import { login } from '$lib/api/auth';
import { auth } from '$lib/stores/auth';
import { harness, makeUser } from '../harness/client';
import { navigations } from '../shims/app-navigation';

let seq = 0;
function teamName(): string {
	seq += 1;
	return `bell-team-${Date.now().toString(36)}-${seq}`;
}

async function asUser(username: string, password: string): Promise<void> {
	auth.login(await login({ username, password }));
}

async function bellButton(name: string) {
	return screen.findByRole('button', { name: `Team invitations and transfers (${name})` });
}

describe('notice bell against the real server', () => {
	it('shows a pending invite and Accept joins the team for real', async () => {
		const h = harness();
		const team = teamName();
		await createTeam(team, h.admin.token);
		const invitee = await makeUser();
		await addTeamMember(team, invitee.username, h.admin.token);

		await asUser(invitee.username, invitee.password);
		render(InviteBell);

		const bell = await bellButton('1');
		expect(bell).toBeTruthy();
		await userEvent.click(bell);

		const accept = await screen.findByRole('button', { name: 'Accept' });
		await userEvent.click(accept);

		// Server-side truth: the membership row flipped to active.
		await waitFor(
			async () => {
				const members = await listTeamMembers(team, h.admin.token);
				expect(members.find((m) => m.username === invitee.username)?.status).toBe('active');
			},
			{ timeout: 5000 }
		);

		// The bell re-fetched on its own and unmounted (nothing pending).
		await waitFor(() => {
			expect(screen.queryByRole('button', { name: /Team invitations and transfers/ })).toBeNull();
		});
	});

	it('decline is a two-step confirm and clears the invite server-side', async () => {
		const h = harness();
		const team = teamName();
		await createTeam(team, h.admin.token);
		const invitee = await makeUser();
		await addTeamMember(team, invitee.username, h.admin.token);

		await asUser(invitee.username, invitee.password);
		render(InviteBell);

		await userEvent.click(await bellButton('1'));
		await userEvent.click(await screen.findByRole('button', { name: 'Decline' }));
		// First click only arms the confirmation.
		expect(screen.queryByRole('button', { name: /Team invitations and transfers \(0\)/ })).toBeNull();
		await userEvent.click(await screen.findByRole('button', { name: 'Confirm decline' }));

		await waitFor(
			async () => {
				const members = await listTeamMembers(team, h.admin.token);
				expect(members.some((m) => m.username === invitee.username)).toBe(false);
			},
			{ timeout: 5000 }
		);
	});

	it('drops entries that are no longer actionable without a poll', async () => {
		const h = harness();
		const team = teamName();
		await createTeam(team, h.admin.token);
		const invitee = await makeUser();
		await addTeamMember(team, invitee.username, h.admin.token);

		await asUser(invitee.username, invitee.password);
		render(InviteBell);
		await bellButton('1');

		// Someone else resolves the invite from the outside. Any bell refresh
		// that happens afterwards (reopen, tick, poll) must observe the new
		// server state and drop the dead entry.
		await declineTeamInvite(team, invitee.token);

		const bell = screen.queryByRole('button', { name: 'Team invitations and transfers (1)' });
		if (bell) {
			// Opening always re-fetches; closing and reopening likewise.
			await userEvent.click(bell);
			const again = screen.queryByRole('button', { name: 'Team invitations and transfers (1)' });
			if (again) await userEvent.click(again);
		}
		await waitFor(
			() => {
				expect(screen.queryByRole('button', { name: /Team invitations and transfers/ })).toBeNull();
			},
			{ timeout: 5000 }
		);

		await waitFor(
			async () => {
				const t = await getTeam(team, h.admin.token);
				expect(t.members ?? []).toEqual([]);
			},
			{ timeout: 5000 }
		);
		// Accepting a revoked invite must fail cleanly rather than join.
		await expect(acceptTeamInvite(team, invitee.token)).rejects.toBeTruthy();
	});

	it('"View all teams" routes to the teams page', async () => {
		const h = harness();
		const team = teamName();
		await createTeam(team, h.admin.token);
		const invitee = await makeUser();
		await addTeamMember(team, invitee.username, h.admin.token);

		await asUser(invitee.username, invitee.password);
		render(InviteBell);

		await userEvent.click(await bellButton('1'));
		await userEvent.click(await screen.findByRole('button', { name: 'View all teams' }));
		expect(navigations).toContain('/teams');
	});
});
