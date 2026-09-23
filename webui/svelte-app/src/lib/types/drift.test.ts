import { describe, expect, it } from 'vitest';
import type { components } from '$lib/api/generated/schema.d';
import type { Paste } from '$lib/types/paste';

// The handwritten interfaces in src/lib/types must stay field-compatible
// with the schemas generated from the server's swagger.yaml. These
// assignments are compile-time assertions; the runtime bodies only guard
// against accidental removal of the types under test.

type SpecPaste = components['schemas']['Paste'];

describe('type drift against generated OpenAPI schema', () => {
	it('Paste stays assignable to the generated schema', () => {
		// Deliberately narrower than the spec: required server fields
		// (id, description, ...) must be present in the local type too,
		// or this assignment fails to compile.
		const local: Paste = {
			id: 1,
			paste_id: 'x',
			name: 'n',
			language: 'go',
			description: '',
			created_by: 'admin',
			owner: 'admin',
			owner_id: 1,
			metadata: {},
			public: false,
			created_at: new Date().toISOString()
		};
		const asSpec: SpecPaste = local;
		expect(asSpec.paste_id).toBe('x');
	});

	it('generated schema keeps every field the UI reads from a paste', () => {
		// If the server ever drops one of these properties the key access
		// below stops type-checking and svelte-check fails the build.
		type SpecKeys = keyof SpecPaste;
		const read: SpecKeys[] = [
			'paste_id',
			'name',
			'language',
			'data',
			'public',
			'created_at',
			'expires',
			'owner',
			'owner_id',
			'labels'
		];
		expect(read.length).toBe(10);
	});
});
