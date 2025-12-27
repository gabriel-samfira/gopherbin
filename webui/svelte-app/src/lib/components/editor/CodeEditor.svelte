<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { EditorView, basicSetup } from 'codemirror';
	import { EditorState, Compartment } from '@codemirror/state';

	export let value = '';
	export let mode = 'text';
	export let theme = 'monokai';
	export let readOnly = false;
	export let onChange: (value: string) => void = () => {};

	let editorContainer: HTMLDivElement;
	let editorView: EditorView | null = null;
	let languageCompartment = new Compartment();
	let themeCompartment = new Compartment();

	// Dynamic language loader
	async function loadLanguage(lang: string) {
		const langLower = lang.toLowerCase();

		try {
			switch (langLower) {
				case 'javascript':
				case 'js':
					return (await import('@codemirror/lang-javascript')).javascript();
				case 'typescript':
				case 'ts':
					return (await import('@codemirror/lang-javascript')).javascript({ typescript: true });
				case 'jsx':
					return (await import('@codemirror/lang-javascript')).javascript({ jsx: true });
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
				case 'cpp':
				case 'c++':
				case 'c':
				case 'c_cpp':
					return (await import('@codemirror/lang-cpp')).cpp();
				case 'java':
					return (await import('@codemirror/lang-java')).java();
				case 'html':
					return (await import('@codemirror/lang-html')).html();
				case 'css':
					return (await import('@codemirror/lang-css')).css();
				case 'sql':
					return (await import('@codemirror/lang-sql')).sql();
				case 'php':
					return (await import('@codemirror/lang-php')).php();
				case 'xml':
					return (await import('@codemirror/lang-xml')).xml();
				case 'json':
					return (await import('@codemirror/lang-json')).json();
				case 'markdown':
				case 'md':
					return (await import('@codemirror/lang-markdown')).markdown();
				default:
					return [];
			}
		} catch (err) {
			console.warn(`Failed to load language ${lang}:`, err);
			return [];
		}
	}

	async function loadTheme(themeName: string) {
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

	onMount(async () => {
		if (editorContainer) {
			const languageExt = await loadLanguage(mode);
			const themeExt = await loadTheme(theme);

			const extensions = [
				basicSetup,
				themeCompartment.of(themeExt),
				languageCompartment.of(languageExt),
				EditorView.updateListener.of((update) => {
					if (update.docChanged) {
						onChange(update.state.doc.toString());
					}
				}),
				EditorState.readOnly.of(readOnly),
				EditorView.theme({
					'&': { height: '450px' },
					'.cm-scroller': { overflow: 'auto' }
				})
			];

			const state = EditorState.create({
				doc: value,
				extensions
			});

			editorView = new EditorView({
				state,
				parent: editorContainer
			});
		}
	});

	onDestroy(() => {
		editorView?.destroy();
	});

	// Update language when mode changes
	$: if (editorView && mode) {
		loadLanguage(mode).then((lang) => {
			editorView?.dispatch({
				effects: languageCompartment.reconfigure(lang)
			});
		});
	}

	// Update theme when theme changes
	$: if (editorView && theme) {
		loadTheme(theme).then((themeExt) => {
			editorView?.dispatch({
				effects: themeCompartment.reconfigure(themeExt)
			});
		});
	}

	// Update content when value changes externally
	$: if (editorView && value !== editorView.state.doc.toString()) {
		editorView.dispatch({
			changes: {
				from: 0,
				to: editorView.state.doc.length,
				insert: value
			}
		});
	}
</script>

<div
	bind:this={editorContainer}
	class="w-full border border-gray-300 dark:border-gray-600 rounded-md overflow-hidden"
></div>

<style>
	:global(.cm-editor) {
		font-size: 14px;
	}
</style>
