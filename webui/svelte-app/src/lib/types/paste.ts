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
	// Mirrors the Paste schema in src/lib/api/generated/schema.d.ts,
	// generated from the server's swagger.yaml. Keep the two in sync.
	// Required fields mirror the server schema, so a server-side change
	// that makes one optional surfaces as a type error here.
	id: number;
	paste_id: string;
	name: string;
	language: string;
	data?: string; // base64 encoded; omitted on list and metadata views
	preview?: string; // base64 encoded preview (first few lines)
	description: string;
	public: boolean;
	created_at: string;
	expires?: string;
	max_accesses?: number;
	access_count?: number;
	created_by: string;
	owner: string;
	owner_id: number;
	team?: string;
	metadata: Record<string, string>;
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
