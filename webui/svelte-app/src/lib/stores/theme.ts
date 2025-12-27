import { writable } from 'svelte/store';
import { browser } from '$app/environment';

export type Theme = 'light' | 'dark';

function createThemeStore() {
	// Initialize from localStorage or system preference
	const getInitialTheme = (): Theme => {
		if (!browser) return 'light';

		const stored = localStorage.getItem('theme');
		if (stored === 'light' || stored === 'dark') {
			return stored;
		}

		// Check system preference
		if (window.matchMedia('(prefers-color-scheme: dark)').matches) {
			return 'dark';
		}

		return 'light';
	};

	const { subscribe, set, update } = writable<Theme>(getInitialTheme());

	// Apply theme to document
	const applyTheme = (theme: Theme) => {
		if (!browser) return;

		if (theme === 'dark') {
			document.documentElement.classList.add('dark');
			document.body.classList.add('dark');
		} else {
			document.documentElement.classList.remove('dark');
			document.body.classList.remove('dark');
		}
	};

	// Initialize theme on load
	if (browser) {
		applyTheme(getInitialTheme());

		// Listen for system preference changes
		const mediaQuery = window.matchMedia('(prefers-color-scheme: dark)');
		mediaQuery.addEventListener('change', (e) => {
			const stored = localStorage.getItem('theme');
			// Only auto-switch if user hasn't set a preference
			if (!stored) {
				const newTheme = e.matches ? 'dark' : 'light';
				set(newTheme);
				applyTheme(newTheme);
			}
		});
	}

	return {
		subscribe,
		toggle: () =>
			update((current) => {
				const newTheme = current === 'light' ? 'dark' : 'light';
				if (browser) {
					localStorage.setItem('theme', newTheme);
					applyTheme(newTheme);
				}
				return newTheme;
			}),
		set: (theme: Theme) => {
			if (browser) {
				localStorage.setItem('theme', theme);
				applyTheme(theme);
			}
			set(theme);
		}
	};
}

export const theme = createThemeStore();
