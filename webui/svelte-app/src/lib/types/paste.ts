export interface PasteLabel {
	name: string;
	color?: string;
	scope?: 'personal' | 'team';
	team?: string;
}

export interface LabelInfo {
	id: number;
	name: string;
	color?: string;
	usage: number;
}

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
	owner?: string;
	owner_id?: number;
	team?: string;
	labels?: PasteLabel[];
}

export interface PasteCreate {
	name: string;
	language: string;
	data: string;
	public: boolean;
	description?: string;
	expires?: Date;
	team?: string;
	metadata?: Record<string, string>;
	labels?: string[];
}

export interface PasteUpdate {
	public?: boolean;
}

export type PasteScope = 'all' | 'mine' | 'shared';

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
