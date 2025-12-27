<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { EditorView, basicSetup } from 'codemirror';
	import { EditorState } from '@codemirror/state';
	import { editorTheme } from '$lib/stores/editorTheme';
	import { get } from 'svelte/store';

	export let content: string;
	export let language: string = 'text';

	let editorContainer: HTMLDivElement;
	let editorView: EditorView | null = null;

	// Get first 10 lines
	const lines = content.split('\n').slice(0, 10);
	const preview = lines.join('\n');
	const hasMore = content.split('\n').length > 10;

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
			const languageExt = await loadLanguage(language);
			const themeName = get(editorTheme);
			const themeExt = await loadTheme(themeName);

			const extensions = [
				EditorView.editable.of(false),
				themeExt,
				languageExt,
				EditorState.readOnly.of(true),
				EditorView.theme({
					'&': {
						height: 'auto',
						maxHeight: '250px'
					},
					'.cm-scroller': {
						overflow: 'auto',
						fontSize: '13px'
					},
					'.cm-gutters': {
						display: 'none'
					}
				})
			];

			const state = EditorState.create({
				doc: preview,
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
</script>

<div class="relative">
	<div bind:this={editorContainer} class="rounded-md overflow-hidden"></div>
	{#if hasMore}
		<div
			class="absolute bottom-0 left-0 right-0 h-12 bg-gradient-to-t from-gray-900 to-transparent flex items-end justify-center pb-2 pointer-events-none"
		>
			<span class="text-gray-400 text-xs">...</span>
		</div>
	{/if}
</div>
