import { apiClient } from './client';
import type { User, UserCreate, UserUpdate, UserList, UserSearchResult } from '$lib/types/user';

export async function listUsers(
	page: number,
	maxResults: number,
	token: string
): Promise<UserList> {
	return apiClient.get<UserList>(`/admin/users?page=${page}&max_results=${maxResults}`, token);
}

export async function getUser(userId: string, token: string): Promise<User> {
	return apiClient.get<User>(`/admin/users/${userId}`, token);
}

export async function createUser(data: UserCreate, token: string): Promise<{ id: string }> {
	return apiClient.post<{ id: string }>('/admin/users', data, token);
}

export async function updateUser(userId: string | number, data: UserUpdate, token: string): Promise<void> {
	return apiClient.put<void>(`/admin/users/${userId}`, data, token);
}

export async function deleteUser(userId: string | number, token: string): Promise<void> {
	return apiClient.delete<void>(`/admin/users/${userId}`, token);
}

export async function searchUsers(q: string, team: string, token: string): Promise<UserSearchResult[]> {
	const params = new URLSearchParams({ q });
	if (team) params.set('team', team);
	return apiClient.get<UserSearchResult[]>(`/users/search?${params.toString()}`, token);
}

export async function getMe(token: string): Promise<User> {
	return apiClient.get<User>('/me', token);
}

export async function updateMe(discoverable: boolean, token: string): Promise<User> {
	return apiClient.put<User>('/me', { discoverable }, token);
}
