<script lang="ts">
	import { page } from '$app/stores';
	import { goto } from '$app/navigation';
	import { onMount } from 'svelte';
	import { auth } from '$lib/stores/auth';
	import { editorTheme } from '$lib/stores/editorTheme';
	import { getPaste } from '$lib/api/pastes';
	import CodeEditor from '$lib/components/editor/CodeEditor.svelte';
	import Button from '$lib/components/ui/Button.svelte';
	import Spinner from '$lib/components/ui/Spinner.svelte';
	import { resolveSyntax } from '$lib/utils/syntax';
	import { timeAgo } from '$lib/utils/date';
	import { copyToClipboard as copyText } from '$lib/utils/clipboard';
	import { formatApiError } from '$lib/utils/errors';
	import { decodeBase64 } from '$lib/utils/base64';
	import type { Paste } from '$lib/types/paste';

	let paste: Paste | null = null;
	let loading = true;
	let error = '';
	let pasteContent = '';
	let showCopyTooltip = false;

	$: pasteId = $page.params.id;

	onMount(async () => {
		if (!$auth.token) {
			// Redirect to login with next parameter
			const currentPath = encodeURIComponent(window.location.pathname);
			goto(`/login?next=${currentPath}`);
			return;
		}

		if (!pasteId) {
			error = 'Invalid paste ID';
			loading = false;
			return;
		}

		try {
			paste = await getPaste(pasteId, $auth.token);
			// Decode base64 content
			pasteContent = decodeBase64(paste.data);
		} catch (err) {
			error = formatApiError(err);
		} finally {
			loading = false;
		}
	});

	async function copyToClipboard() {
		if (pasteContent) {
			try {
				await copyText(pasteContent);
				showCopyTooltip = true;
				setTimeout(() => {
					showCopyTooltip = false;
				}, 1000);
			} catch (err) {
				error = 'Failed to copy to clipboard';
			}
		}
	}
</script>

<svelte:head>
	<title>{paste?.name || 'Paste'} - GopherBin</title>
</svelte:head>

{#if loading}
	<Spinner />
{:else if error}
	<div class="p-4 bg-red-100 dark:bg-red-900 text-red-700 dark:text-red-200 rounded-md">
		{error}
	</div>
{:else if paste}
	<div class="space-y-4">
		<div class="flex flex-col sm:flex-row sm:justify-between sm:items-start gap-4">
			<div class="flex-1">
				<h1 class="text-2xl sm:text-3xl font-bold text-gray-900 dark:text-gray-100 break-words">{paste.name}</h1>
				<p class="text-sm sm:text-base text-gray-600 dark:text-gray-400 mt-2">
					Created
					{#if paste.created_by}
						by <strong>{paste.created_by}</strong>
					{/if}
					{timeAgo(paste.created_at)}
				</p>
			</div>

			<div class="relative">
				<Button on:click={copyToClipboard} variant="secondary" class="w-full sm:w-auto">
					Copy
				</Button>
				{#if showCopyTooltip}
					<div
						class="absolute top-full mt-2 right-0 px-3 py-1 bg-gray-800 dark:bg-gray-700 text-white text-sm rounded-md"
					>
						Copied!
					</div>
				{/if}
			</div>
		</div>

		<CodeEditor
			value={pasteContent}
			mode={resolveSyntax(paste.language)}
			theme={$editorTheme}
			readOnly={true}
			dynamicHeight={true}
		/>
	</div>
{/if}
