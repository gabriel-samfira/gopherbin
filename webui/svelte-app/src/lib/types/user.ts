export interface User {
	id: number;
	username: string;
	email: string;
	full_name: string;
	enabled: boolean;
	is_admin: boolean;
	is_superuser: boolean;
	discoverable: boolean;
	created_at: string;
	updated_at: string;
}

export interface UserCreate {
	username: string;
	email: string;
	password: string;
	full_name: string;
	enabled?: boolean;
	is_admin?: boolean;
}

export interface UserUpdate {
	username?: string;
	email?: string;
	full_name?: string;
	enabled?: boolean;
	is_admin?: boolean;
	password?: string;
	/**
	 * Required by the server whenever `password` is set and the caller is
	 * updating their own account (see PUT /api/v1/admin/users/{id}).
	 */
	current_password?: string;
	discoverable?: boolean;
}

export interface UserSearchResult {
	id: number;
	username: string;
	full_name: string;
}

export interface UserList {
	users: User[];
	page: number;
	total_pages: number;
	max_results: number;
}
