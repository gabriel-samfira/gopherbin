import { apiClient } from './client';
import type { Paste, PasteCreate, PasteList, PasteScope, PasteUpdate, PasteShare, LabelInfo } from '$lib/types/paste';

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
	token: string,
	scope: PasteScope = 'all',
	labels: string[] = [],
	team = ''
): Promise<PasteList> {
	return apiClient.get<PasteList>(
		`/paste?${pasteQueryParams(page, maxResults, scope, labels, team)}`,
		token
	);
}

function pasteQueryParams(
	page: number,
	maxResults: number,
	scope: PasteScope,
	labels: string[],
	team: string
): string {
	const params = new URLSearchParams({
		page: String(page),
		max_results: String(maxResults),
		scope
	});
	if (labels.length > 0) params.set('labels', labels.join(','));
	if (team) params.set('team', team);
	return params.toString();
}

export async function searchPastes(
	query: string,
	page: number,
	maxResults: number,
	token: string,
	scope: PasteScope = 'all',
	labels: string[] = [],
	team = ''
): Promise<PasteList> {
	return apiClient.get<PasteList>(
		`/paste/search?q=${encodeURIComponent(query)}&${pasteQueryParams(page, maxResults, scope, labels, team)}`,
		token
	);
}

export interface LabelVocabulary {
	personal: string[];
	teams: { team: string; labels: string[] }[];
	colors?: Record<string, string>;
}

export async function getLabelVocabulary(token: string): Promise<LabelVocabulary> {
	return apiClient.get<LabelVocabulary>('/labels', token);
}

export async function setPasteLabels(pasteId: string, labels: string[], token: string): Promise<void> {
	await apiClient.put(`/paste/${pasteId}/labels`, { labels }, token);
}

export async function getMyLabels(token: string): Promise<LabelInfo[]> {
	return apiClient.get<LabelInfo[]>('/labels/mine', token);
}

export async function updateLabel(
	labelId: number,
	args: { name?: string; color?: string },
	token: string
): Promise<LabelInfo> {
	return apiClient.put<LabelInfo>(`/labels/${labelId}`, args, token);
}

export async function deleteLabel(labelId: number, token: string): Promise<void> {
	await apiClient.delete<void>(`/labels/${labelId}`, token);
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

export async function transferPaste(
	pasteId: string,
	userID: string,
	token: string
): Promise<Paste> {
	return apiClient.post<Paste>(`/paste/${pasteId}/transfer`, { userID }, token);
}
