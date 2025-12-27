export interface User {
	id: string;
	username: string;
	email: string;
	full_name: string;
	enabled: boolean;
	is_admin: boolean;
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
}

export interface UserList {
	users: User[];
	page: number;
	total_pages: number;
	max_results: number;
}
