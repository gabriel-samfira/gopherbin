<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { EditorView, basicSetup } from 'codemirror';
	import { EditorState } from '@codemirror/state';
	import { editorTheme } from '$lib/stores/editorTheme';
	import { get } from 'svelte/store';
	import { loadLanguage, loadTheme } from '$lib/utils/cmLanguages';

	export let content: string;
	export let language: string = 'text';

	let editorContainer: HTMLDivElement;
	let editorView: EditorView | null = null;

	// Get first 10 lines
	const lines = content.split('\n').slice(0, 10);
	const preview = lines.join('\n');
	const hasMore = content.split('\n').length > 10;

	// Language and theme loading shared with CodeEditor via $lib/utils/cmLanguages

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
						overflowY: 'auto',
						overflowX: 'hidden',
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
