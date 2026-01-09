<script lang="ts">
	import { goto } from '$app/navigation';
	import { auth } from '$lib/stores/auth';
	import { editorTheme } from '$lib/stores/editorTheme';
	import { createPaste } from '$lib/api/pastes';
	import CodeEditor from '$lib/components/editor/CodeEditor.svelte';
	import Button from '$lib/components/ui/Button.svelte';
	import Input from '$lib/components/ui/Input.svelte';
	import PrivacyToggle from '$lib/components/ui/PrivacyToggle.svelte';
	import Spinner from '$lib/components/ui/Spinner.svelte';
	import { getLanguageFromFilename, editorThemes } from '$lib/utils/syntax';
	import { formatApiError } from '$lib/utils/errors';
	import { encodeBase64 } from '$lib/utils/base64';

	let filename = '';
	let content = '';
	let isPublic = false;
	let language = 'text';
	let expiresDate: string = '';
	let loading = false;
	let error = '';

	$: canSubmit = filename.length > 0 && content.length > 0;

	// Auto-detect language from filename
	$: if (filename) {
		language = getLanguageFromFilename(filename);
	}

	function handleContentChange(newContent: string) {
		content = newContent;
	}

	async function handleSubmit(e: Event) {
		e.preventDefault();

		if (!canSubmit || !$auth.token) return;

		loading = true;
		error = '';

		try {
			const pasteData = {
				name: filename,
				language: language,
				data: encodeBase64(content),
				public: isPublic,
				description: '',
				...(expiresDate && { expires: new Date(expiresDate) })
			};

			const response = await createPaste(pasteData, $auth.token);

			// Redirect to the created paste
			const redirectPath = isPublic ? `/public/p/${response.paste_id}` : `/p/${response.paste_id}`;
			goto(redirectPath);
		} catch (err) {
			error = formatApiError(err);
		} finally {
			loading = false;
		}
	}

	// Redirect if not authenticated
	$: if (!$auth.isAuthenticated) {
		const currentPath = encodeURIComponent(window.location.pathname);
		goto(`/login?next=${currentPath}`);
	}
</script>

<svelte:head>
	<title>New Paste - GopherBin</title>
</svelte:head>

{#if !$auth.isAuthenticated}
	<Spinner />
{:else}
	<div class="space-y-6">
		<h1 class="text-2xl sm:text-3xl font-bold text-gray-900 dark:text-gray-100">Create New Paste</h1>

		{#if error}
			<div class="p-3 bg-red-100 dark:bg-red-900 text-red-700 dark:text-red-200 rounded-md">
				{error}
			</div>
		{/if}

		{#if loading}
			<Spinner />
		{:else}
			<form on:submit={handleSubmit} class="space-y-4">
				<div class="flex flex-col sm:flex-row sm:flex-wrap gap-3 sm:gap-4 sm:items-center">
					<div class="flex-1 min-w-full sm:min-w-[200px]">
						<Input
							bind:value={filename}
							placeholder="File name (e.g., myfile.js)"
						/>
					</div>

					<div class="flex gap-3 flex-wrap items-center">
						<PrivacyToggle bind:isPublic />

						<select
							bind:value={$editorTheme}
							class="px-3 py-2 border rounded-md bg-white dark:bg-gray-800 text-gray-900 dark:text-gray-100 border-gray-300 dark:border-gray-600 text-sm"
						>
							{#each editorThemes as themeOption}
								<option value={themeOption}>
									{themeOption.replaceAll('_', ' ')}
								</option>
							{/each}
						</select>

						<input
							type="datetime-local"
							bind:value={expiresDate}
							class="px-3 py-2 border rounded-md bg-white dark:bg-gray-800 text-gray-900 dark:text-gray-100 border-gray-300 dark:border-gray-600 text-sm"
							placeholder="Expires..."
						/>
					</div>
				</div>

				<CodeEditor value={content} mode={language} theme={$editorTheme} onChange={handleContentChange} />

				<div>
					<Button type="submit" variant="success" disabled={!canSubmit} class="w-full sm:w-auto">
						Submit
					</Button>
				</div>
			</form>
		{/if}
	</div>
{/if}
