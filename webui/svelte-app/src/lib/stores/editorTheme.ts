import { writable } from 'svelte/store';
import { browser } from '$app/environment';
import { defaultEditorTheme } from '$lib/utils/syntax';

const STORAGE_KEY = 'editorTheme';

function createEditorThemeStore() {
	const getInitialTheme = (): string => {
		if (!browser) return defaultEditorTheme;

		const stored = localStorage.getItem(STORAGE_KEY);
		return stored || defaultEditorTheme;
	};

	const { subscribe, set } = writable<string>(getInitialTheme());

	return {
		subscribe,
		set: (theme: string) => {
			if (browser) {
				localStorage.setItem(STORAGE_KEY, theme);
			}
			set(theme);
		}
	};
}

export const editorTheme = createEditorThemeStore();
