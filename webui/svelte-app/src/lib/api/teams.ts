import { apiClient } from './client';
import { refreshNotices } from '$lib/stores/notices';
import type { Team, TeamList, TeamMember, TeamInviteInfo, TeamTransferInfo, MemberRole } from '$lib/types/team';

export async function listTeams(page: number, maxResults: number, token: string): Promise<TeamList> {
	return apiClient.get<TeamList>(`/teams?page=${page}&max_results=${maxResults}`, token);
}

export async function getTeam(name: string, token: string): Promise<Team> {
	return apiClient.get<Team>(`/teams/${encodeURIComponent(name)}`, token);
}

export async function createTeam(name: string, token: string, description = ''): Promise<Team> {
	return apiClient.post<Team>('/teams', { name, description }, token);
}

export async function updateTeam(
	name: string,
	update: { name?: string; description?: string },
	token: string
): Promise<Team> {
	return apiClient.put<Team>(`/teams/${encodeURIComponent(name)}`, update, token);
}

export async function setTeamLabels(name: string, labels: string[], token: string): Promise<Team> {
	return apiClient.put<Team>(`/teams/${encodeURIComponent(name)}/labels`, { labels }, token);
}

export async function deleteTeam(name: string, token: string): Promise<void> {
	return apiClient.delete<void>(`/teams/${encodeURIComponent(name)}`, token);
}

export async function listTeamMembers(name: string, token: string): Promise<TeamMember[]> {
	return apiClient.get<TeamMember[]>(`/teams/${encodeURIComponent(name)}/members`, token);
}

export async function addTeamMember(
	name: string,
	userID: string,
	token: string,
	role?: MemberRole
): Promise<TeamMember> {
	return apiClient.post<TeamMember>(
		`/teams/${encodeURIComponent(name)}/members`,
		{ userID, role },
		token
	);
}

export async function setTeamMemberRole(
	name: string,
	member: string,
	role: MemberRole,
	token: string
): Promise<TeamMember> {
	return apiClient.put<TeamMember>(
		`/teams/${encodeURIComponent(name)}/members/${encodeURIComponent(member)}`,
		{ role },
		token
	);
}

export async function transferTeam(name: string, userID: string, token: string): Promise<Team> {
	return apiClient.post<Team>(`/teams/${encodeURIComponent(name)}/transfer`, { userID }, token);
}

export async function teamTransferAction(
	name: string,
	action: 'accept' | 'decline' | 'cancel',
	token: string
): Promise<Team | void> {
	try {
		return await apiClient.post<Team>(
			`/teams/${encodeURIComponent(name)}/transfer/${action}`,
			{},
			token
		);
	} finally {
		refreshNotices();
	}
}

export async function listPendingTransfers(token: string): Promise<TeamTransferInfo[]> {
	return apiClient.get<TeamTransferInfo[]>('/teams/transfers', token);
}

export async function removeTeamMember(name: string, userID: string, token: string): Promise<void> {
	return apiClient.delete<void>(
		`/teams/${encodeURIComponent(name)}/members/${encodeURIComponent(userID)}`,
		token
	);
}

export async function listPendingInvites(token: string): Promise<TeamInviteInfo[]> {
	return apiClient.get<TeamInviteInfo[]>('/teams/invites', token);
}

export async function acceptTeamInvite(name: string, token: string): Promise<Team> {
	try {
		return await apiClient.post<Team>(`/teams/${encodeURIComponent(name)}/accept`, {}, token);
	} finally {
		// Even a failed accept (e.g. the invite was revoked meanwhile) means
		// the notice state on the server may have changed.
		refreshNotices();
	}
}

export async function declineTeamInvite(name: string, token: string): Promise<void> {
	try {
		await apiClient.post<void>(`/teams/${encodeURIComponent(name)}/decline`, {}, token);
	} finally {
		refreshNotices();
	}
}

export async function leaveTeam(name: string, token: string): Promise<void> {
	return apiClient.post<void>(`/teams/${encodeURIComponent(name)}/leave`, {}, token);
}
