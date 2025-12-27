import type { ApiError } from '$lib/types/api';

/**
 * Formats an API error into a user-friendly message
 */
export function formatApiError(err: unknown): string {
	const apiError = err as ApiError;

	// If we have details, prefer that over the generic error message
	if (apiError.details && apiError.details !== apiError.error) {
		return apiError.details;
	}

	// If we have an error message, use it
	if (apiError.error) {
		return apiError.error;
	}

	// Fallback based on status code
	if (apiError.status) {
		switch (apiError.status) {
			case 400:
				return 'Bad request';
			case 401:
				return 'Unauthorized - please log in again';
			case 403:
				return 'Forbidden - you do not have permission to access this resource';
			case 404:
				return 'Not found';
			case 409:
				return 'Conflict - the resource already exists';
			case 422:
				return 'Validation failed';
			case 429:
				return 'Too many requests - please try again later';
			case 500:
				return 'Server error - please try again later';
			case 502:
				return 'Bad gateway - the server is temporarily unavailable';
			case 503:
				return 'Service unavailable - please try again later';
			default:
				return `Request failed with status ${apiError.status}`;
		}
	}

	// Network error fallback
	return 'Failed to connect to server';
}
