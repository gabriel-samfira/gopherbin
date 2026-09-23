<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { EditorView } from 'codemirror';
	import { pasteSetup } from '$lib/utils/cmSetup';
	import { EditorState, Compartment, Prec } from '@codemirror/state';
	import { indentWithTab } from '@codemirror/commands';
	import { keymap } from '@codemirror/view';
	import { loadLanguage, loadTheme } from '$lib/utils/cmLanguages';

	export let value = '';
	export let mode = 'text';
	export let theme = 'monokai';
	export let readOnly = false;
	export let onChange: (value: string) => void = () => {};
	export let dynamicHeight = false; // Enable dynamic height for read-only views

	let editorContainer: HTMLDivElement;
	let editorView: EditorView | null = null;
	let languageCompartment = new Compartment();
	let themeCompartment = new Compartment();
	let viewportHeight = 0;

	const handleResize = () => {
		if (dynamicHeight) {
			viewportHeight = window.innerHeight - 225;
			if (editorView) {
				editorView.dom.style.maxHeight = `${viewportHeight}px`;
			}
		}
	};

	// Language and theme loading shared with PastePreview via $lib/utils/cmLanguages
	onMount(async () => {
		if (editorContainer) {
			const languageExt = await loadLanguage(mode);
			const themeExt = await loadTheme(theme);

			// Configure height based on dynamicHeight prop
			let heightConfig;
			if (dynamicHeight) {
				// Calculate viewport height minus header and padding for dynamic mode
				viewportHeight = window.innerHeight - 225; // Reserve space for header and margins
				heightConfig = {
					'&': {
						height: 'auto',
						maxHeight: `${viewportHeight}px`
					},
					'.cm-scroller': { overflow: 'auto' }
				};
			} else {
				// Fixed height for editing mode
				heightConfig = {
					'&': { height: '450px' },
					'.cm-scroller': { overflow: 'auto' }
				};
			}

			const extensions = [
				pasteSetup,
				// Keep Tab inside the editor when editable; in readonly mode
				// let it move focus as usual.
				...(readOnly ? [] : [Prec.high(keymap.of([indentWithTab]))]),
				themeCompartment.of(themeExt),
				languageCompartment.of(languageExt),
				EditorView.updateListener.of((update) => {
					if (update.docChanged) {
						onChange(update.state.doc.toString());
					}
				}),
				EditorState.readOnly.of(readOnly),
				EditorView.lineWrapping,
				EditorView.theme(heightConfig)
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

		window.addEventListener('resize', handleResize);
	});

	onDestroy(() => {
		window.removeEventListener('resize', handleResize);
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
