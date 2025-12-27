import { apiClient } from './client';
import type { Paste, PasteCreate, PasteList, PasteUpdate, PasteShare } from '$lib/types/paste';

export async function createPaste(data: PasteCreate, token: string): Promise<{ paste_id: string }> {
	return apiClient.post<{ paste_id: string }>('/paste', data, token);
}

export async function getPaste(pasteId: string, token: string): Promise<Paste> {
	return apiClient.get<Paste>(`/paste/${pasteId}`, token);
}

export async function getPublicPaste(pasteId: string): Promise<Paste> {
	return apiClient.get<Paste>(`/public/paste/${pasteId}`);
}

export async function listPastes(
	page: number,
	maxResults: number,
	token: string
): Promise<PasteList> {
	return apiClient.get<PasteList>(`/paste?page=${page}&max_results=${maxResults}`, token);
}

export async function searchPastes(
	query: string,
	page: number,
	maxResults: number,
	token: string
): Promise<PasteList> {
	return apiClient.get<PasteList>(`/paste/search?q=${encodeURIComponent(query)}&page=${page}&max_results=${maxResults}`, token);
}

export async function updatePaste(
	pasteId: string,
	data: PasteUpdate,
	token: string
): Promise<void> {
	return apiClient.put<void>(`/paste/${pasteId}`, data, token);
}

export async function deletePaste(pasteId: string, token: string): Promise<void> {
	return apiClient.delete<void>(`/paste/${pasteId}`, token);
}

export async function listPasteShares(pasteId: string, token: string): Promise<PasteShare[]> {
	const response = await apiClient.get<{ users: PasteShare[] }>(`/paste/${pasteId}/sharing`, token);
	return response.users || [];
}

export async function sharePaste(
	pasteId: string,
	username: string,
	token: string
): Promise<void> {
	return apiClient.post<void>(`/paste/${pasteId}/sharing`, { userID: username }, token);
}

export async function unsharePaste(
	pasteId: string,
	username: string,
	token: string
): Promise<void> {
	return apiClient.delete<void>(`/paste/${pasteId}/sharing/${username}`, token);
}
