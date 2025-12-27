// Language detection from file extension
export const extensionToLanguage: Record<string, string> = {
	// Programming languages
	js: 'javascript',
	jsx: 'javascript',
	ts: 'typescript',
	tsx: 'typescript',
	py: 'python',
	rb: 'ruby',
	java: 'java',
	c: 'c_cpp',
	cpp: 'c_cpp',
	cc: 'c_cpp',
	cxx: 'c_cpp',
	h: 'c_cpp',
	hpp: 'c_cpp',
	cs: 'csharp',
	php: 'php',
	go: 'golang',
	rs: 'rust',
	swift: 'swift',
	kt: 'kotlin',
	scala: 'scala',
	r: 'r',
	m: 'objectivec',
	lua: 'lua',
	pl: 'perl',
	sh: 'sh',
	bash: 'sh',
	zsh: 'sh',
	fish: 'sh',
	ps1: 'powershell',
	dart: 'dart',

	// Web
	html: 'html',
	htm: 'html',
	xml: 'xml',
	css: 'css',
	scss: 'scss',
	sass: 'sass',
	less: 'less',

	// Data/Config
	json: 'json',
	yaml: 'yaml',
	yml: 'yaml',
	toml: 'toml',
	ini: 'ini',
	conf: 'ini',
	config: 'ini',

	// Markup/Docs
	md: 'markdown',
	markdown: 'markdown',
	rst: 'rst',
	tex: 'latex',

	// Database
	sql: 'sql',

	// Other
	dockerfile: 'dockerfile',
	makefile: 'makefile',
	gitignore: 'gitignore',
	txt: 'text',
	log: 'text'
};

export const fallbackEditorMode = 'text';

// CodeMirror 6 themes (using thememirror package + @codemirror/theme-one-dark)
export const editorThemes = [
	// Dark themes
	'one_dark',
	'dracula',
	'cobalt',
	'bespin',
	'birds_of_paradise',
	'espresso',
	'amy',
	'barf',
	'boys_and_girls',
	'cool_glow',
	'noctis_lilac',
	'smoothy',
	// Light themes
	'ayu_light',
	'solarized_light',
	'tomorrow',
	'clouds',
	'rose_pine_dawn'
];

export const defaultEditorTheme = 'one_dark';

export function getLanguageFromFilename(filename: string): string {
	if (!filename || filename.length < 2 || !filename.includes('.')) {
		return fallbackEditorMode;
	}

	const extension = filename.split('.').pop()?.toLowerCase();
	if (!extension) {
		return fallbackEditorMode;
	}

	return extensionToLanguage[extension] || fallbackEditorMode;
}

export function resolveSyntax(language: string): string {
	return language || fallbackEditorMode;
}

export function editorTheme(themeName: string) {
	return {
		value: themeName,
		label: themeName.replaceAll('_', ' ')
	};
}
