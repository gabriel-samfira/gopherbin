export interface ApiError {
	error: string;
	details: string;
	status: number;
}

export interface LoginRequest {
	username: string;
	password: string;
}

export interface LoginResponse {
	token: string;
}

export interface JWTPayload {
	user: number;
	updated_at: string;
	token_id: string;
	full_name: string;
	is_admin: boolean;
	is_superuser: boolean;
	exp: number;
	iss: string;
}

export interface ApiResponse<T> {
	data?: T;
	error?: ApiError;
}
