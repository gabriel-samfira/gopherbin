import { apiClient } from './client';
import type { LoginRequest, LoginResponse } from '$lib/types/api';

export async function login(credentials: LoginRequest): Promise<LoginResponse> {
	return apiClient.post<LoginResponse>('/auth/login', credentials);
}

export async function logout(token: string): Promise<void> {
	return apiClient.get<void>('/logout', token);
}
