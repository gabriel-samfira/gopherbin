import type { ApiError } from '$lib/types/api';

const API_BASE = '/api/v1';

export class ApiClient {
	private async request<T>(
		endpoint: string,
		options: RequestInit = {},
		token?: string | null
	): Promise<T> {
		const headers: HeadersInit = {
			'Content-Type': 'application/json',
			...options.headers
		};

		if (token) {
			headers['Authorization'] = `Bearer ${token}`;
		}

		const url = `${API_BASE}${endpoint}`;

		try {
			const response = await fetch(url, {
				...options,
				headers
			});

			if (!response.ok) {
				const error: ApiError = await response.json().catch(() => ({
					error: 'Unknown error',
					details: response.statusText,
					status: response.status
				}));

				throw error;
			}

			// Handle empty responses (204 No Content, or empty body)
			if (response.status === 204 || response.headers.get('content-length') === '0') {
				return {} as T;
			}

			// Check if response has content
			const text = await response.text();
			if (!text) {
				return {} as T;
			}

			// Try to parse as JSON
			try {
				return JSON.parse(text);
			} catch {
				return {} as T;
			}
		} catch (error) {
			if ((error as ApiError).status) {
				throw error;
			}

			throw {
				error: 'Network error',
				details: (error as Error).message || 'Failed to connect to server',
				status: 0
			} as ApiError;
		}
	}

	async get<T>(endpoint: string, token?: string | null): Promise<T> {
		return this.request<T>(endpoint, { method: 'GET' }, token);
	}

	async post<T>(endpoint: string, data?: unknown, token?: string | null): Promise<T> {
		return this.request<T>(
			endpoint,
			{
				method: 'POST',
				body: data ? JSON.stringify(data) : undefined
			},
			token
		);
	}

	async put<T>(endpoint: string, data?: unknown, token?: string | null): Promise<T> {
		return this.request<T>(
			endpoint,
			{
				method: 'PUT',
				body: data ? JSON.stringify(data) : undefined
			},
			token
		);
	}

	async delete<T>(endpoint: string, token?: string | null): Promise<T> {
		return this.request<T>(endpoint, { method: 'DELETE' }, token);
	}
}

export const apiClient = new ApiClient();
