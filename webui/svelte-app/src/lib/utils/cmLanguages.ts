// Shared CodeMirror language/theme loaders, used by the editor, the paste
// view and list previews so all three highlight identically.

// Dynamic language loader
export async function loadLanguage(lang: string) {
	const langLower = lang.toLowerCase();

	try {
		switch (langLower) {
			case 'javascript':
			case 'js':
			case 'mjs':
			case 'cjs':
				return (await import('@codemirror/lang-javascript')).javascript();
			case 'jsx':
				return (await import('@codemirror/lang-javascript')).javascript({ jsx: true });
			case 'typescript':
			case 'ts':
			case 'mts':
			case 'cts':
				return (await import('@codemirror/lang-javascript')).javascript({ typescript: true });
			case 'tsx':
				return (await import('@codemirror/lang-javascript')).javascript({ typescript: true, jsx: true });
			case 'python':
			case 'py':
				return (await import('@codemirror/lang-python')).python();
			case 'go':
			case 'golang':
				return (await import('@codemirror/lang-go')).go();
			case 'rust':
			case 'rs':
				return (await import('@codemirror/lang-rust')).rust();
			case 'java':
				return (await import('@codemirror/lang-java')).java();
			case 'cpp':
			case 'c++':
			case 'c':
			case 'c_cpp':
				return (await import('@codemirror/lang-cpp')).cpp();
			case 'php':
				return (await import('@codemirror/lang-php')).php();
			case 'sql':
				return (await import('@codemirror/lang-sql')).sql();
			case 'css':
				return (await import('@codemirror/lang-css')).css();
			case 'html':
			case 'htm':
			case 'xhtml':
				return (await import('@codemirror/lang-html')).html();
			case 'xml':
				return (await import('@codemirror/lang-xml')).xml();
			case 'json':
				return (await import('@codemirror/lang-json')).json();
			case 'markdown':
			case 'md':
				return (await import('@codemirror/lang-markdown')).markdown();
			case 'yaml':
			case 'yml':
				return (await import('@codemirror/lang-yaml')).yaml();
			case 'shell':
			case 'sh':
			case 'bash':
			case 'zsh':
			case 'console': {
				const [{ StreamLanguage }, { shell }] = await Promise.all([
					import('@codemirror/language'),
					import('@codemirror/legacy-modes/mode/shell')
				]);
				return StreamLanguage.define(shell);
			}
			default:
				return [];
		}
	} catch (err) {
		console.warn(`Failed to load language ${lang}:`, err);
		return [];
	}
}

export async function loadTheme(themeName: string) {
	try {
		switch (themeName) {
			case 'one_dark':
				return (await import('@codemirror/theme-one-dark')).oneDark;
			case 'dracula':
				return (await import('thememirror')).dracula;
			case 'cobalt':
				return (await import('thememirror')).cobalt;
			case 'bespin':
				return (await import('thememirror')).bespin;
			case 'birds_of_paradise':
				return (await import('thememirror')).birdsOfParadise;
			case 'espresso':
				return (await import('thememirror')).espresso;
			case 'amy':
				return (await import('thememirror')).amy;
			case 'barf':
				return (await import('thememirror')).barf;
			case 'boys_and_girls':
				return (await import('thememirror')).boysAndGirls;
			case 'cool_glow':
				return (await import('thememirror')).coolGlow;
			case 'noctis_lilac':
				return (await import('thememirror')).noctisLilac;
			case 'smoothy':
				return (await import('thememirror')).smoothy;
			case 'ayu_light':
				return (await import('thememirror')).ayuLight;
			case 'solarized_light':
				return (await import('thememirror')).solarizedLight;
			case 'tomorrow':
				return (await import('thememirror')).tomorrow;
			case 'clouds':
				return (await import('thememirror')).clouds;
			case 'rose_pine_dawn':
				return (await import('thememirror')).rosePineDawn;
			default:
				return (await import('@codemirror/theme-one-dark')).oneDark;
		}
	} catch (err) {
		console.warn(`Failed to load theme ${themeName}:`, err);
		return (await import('@codemirror/theme-one-dark')).oneDark;
	}
}
