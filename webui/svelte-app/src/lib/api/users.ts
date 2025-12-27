import { apiClient } from './client';
import type { User, UserCreate, UserUpdate, UserList } from '$lib/types/user';

export async function listUsers(
	page: number,
	maxResults: number,
	token: string
): Promise<UserList> {
	return apiClient.get<UserList>(`/admin/users?page=${page}&max_results=${maxResults}`, token);
}

export async function createUser(data: UserCreate, token: string): Promise<{ id: string }> {
	return apiClient.post<{ id: string }>('/admin/users', data, token);
}

export async function updateUser(userId: string, data: UserUpdate, token: string): Promise<void> {
	return apiClient.put<void>(`/admin/users/${userId}`, data, token);
}

export async function deleteUser(userId: string, token: string): Promise<void> {
	return apiClient.delete<void>(`/admin/users/${userId}`, token);
}
