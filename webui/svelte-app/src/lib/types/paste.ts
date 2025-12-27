export interface Paste {
	paste_id: string;
	name: string;
	language: string;
	data: string; // base64 encoded
	preview?: string; // base64 encoded preview (first few lines)
	public: boolean;
	created_at: string;
	updated_at: string;
	expires?: string;
	created_by?: string;
}

export interface PasteCreate {
	name: string;
	language: string;
	data: string;
	public: boolean;
	description?: string;
	expires?: Date;
	metadata?: Record<string, string>;
}

export interface PasteUpdate {
	public?: boolean;
}

export interface PasteList {
	pastes: Paste[];
	page: number;
	total_pages: number;
	max_results: number;
}

export interface PasteShare {
	username: string;
	full_name: string;
}
