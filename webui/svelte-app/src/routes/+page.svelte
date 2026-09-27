<script lang="ts">
	import { goto } from '$app/navigation';
	import { browser } from '$app/environment';
	import { page } from '$app/stores';
	import { onMount } from 'svelte';
	import { auth } from '$lib/stores/auth';
	import { editorTheme } from '$lib/stores/editorTheme';
	import { createPaste, getLabelVocabulary } from '$lib/api/pastes';
	import LabelInput from '$lib/components/ui/LabelInput.svelte';
	import { listTeams } from '$lib/api/teams';
	import CodeEditor from '$lib/components/editor/CodeEditor.svelte';
	import Button from '$lib/components/ui/Button.svelte';
	import Input from '$lib/components/ui/Input.svelte';
	import PrivacyToggle from '$lib/components/ui/PrivacyToggle.svelte';
	import Spinner from '$lib/components/ui/Spinner.svelte';
	import { getLanguageFromFilename, editorThemes } from '$lib/utils/syntax';
	import { formatApiError } from '$lib/utils/errors';
	import { encodeBase64 } from '$lib/utils/base64';
	import type { Team } from '$lib/types/team';
	import { Lock } from 'lucide-svelte';

	let filename = '';
	let content = '';
	let isPublic = false;
	let language = 'text';
	let expiresDate: string = '';
	let loading = false;
	let error = '';
	let teams: Team[] = [];
	let selectedTeam = '';
	let pasteLabels: string[] = [];
	let personalLabels: string[] = [];
	let vocabTeams: { team: string; labels: string[] }[] = [];
	let vocabColors: Record<string, string> = {};

	$: labelSuggestions = selectedTeam
		? vocabTeams.find((t) => t.team === selectedTeam)?.labels || []
		: personalLabels;

	$: canSubmit = filename.length > 0 && content.length > 0;

	// Team pastes are always private; access follows team membership
	$: if (selectedTeam) {
		isPublic = false;
	}

	let lastTeam = '';
	$: if (selectedTeam !== lastTeam) {
		lastTeam = selectedTeam;
		pasteLabels = [];
	}

	// Auto-detect language from filename
	$: if (filename) {
		language = getLanguageFromFilename(filename);
	}

	onMount(async () => {
		if (!$auth.token) return;
		try {
			const res = await listTeams(1, 100, $auth.token);
			// Only teams the user may post to: not pending invitations, not
			// read-only viewer memberships.
			teams = (res.teams || []).filter((t) => ['owner', 'admin', 'member'].includes(t.my_role ?? ''));
		} catch {
			// Team features are optional; ignore load failures
		}
		try {
			const vocab = await getLabelVocabulary($auth.token);
			personalLabels = vocab.personal || [];
			vocabTeams = vocab.teams || [];
			vocabColors = vocab.colors || {};
		} catch {
			// Labels are optional; ignore load failures
		}
	});

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
				public: selectedTeam ? false : isPublic,
				description: '',
				...(selectedTeam && { team: selectedTeam }),
				...(expiresDate && { expires: new Date(expiresDate) }),
				...(pasteLabels.length > 0 && { labels: pasteLabels })
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
	$: if (browser && !$auth.isAuthenticated) {
		const currentPath = encodeURIComponent($page.url.pathname);
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
					<div class="flex-1 min-w-full sm:min-w-[200px] [&_input]:h-10">
						<Input
							bind:value={filename}
							placeholder="File name (e.g., myfile.js)"
						/>
					</div>

				<div class="flex gap-3 flex-wrap items-center">
					{#if teams.length > 0}
						<select
							bind:value={selectedTeam}
							class="px-3 h-10 border rounded-md bg-white dark:bg-gray-800 text-gray-900 dark:text-gray-100 border-gray-300 dark:border-gray-600 text-sm"
							title="Team"
						>
							<option value="">Personal</option>
							{#each teams as team}
								<option value={team.name}>{team.name}</option>
							{/each}
						</select>
					{/if}

					{#if selectedTeam}
						<span
							class="flex items-center gap-1 text-sm text-gray-700 dark:text-gray-300 px-2 py-1 bg-gray-100 dark:bg-gray-700 rounded-md"
							title="Team pastes are always private and visible to all team members"
						>
							<Lock class="w-4 h-4" />
							Team: {selectedTeam}
						</span>
					{:else}
						<PrivacyToggle bind:isPublic />
					{/if}

					<select
						bind:value={$editorTheme}
						class="px-3 h-10 border rounded-md bg-white dark:bg-gray-800 text-gray-900 dark:text-gray-100 border-gray-300 dark:border-gray-600 text-sm"
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
						class="px-3 h-10 border rounded-md bg-white dark:bg-gray-800 text-gray-900 dark:text-gray-100 border-gray-300 dark:border-gray-600 text-sm"
						placeholder="Expires..."
					/>
				</div>
				</div>

				<div>
					<div class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
						Labels
						<span class="text-xs font-normal text-gray-500 dark:text-gray-400 ml-1">
							{selectedTeam ? 'team labels — shared with the team' : 'personal labels — only visible to you'}
						</span>
					</div>
					<LabelInput
						labels={pasteLabels}
						suggestions={labelSuggestions}
						colorMap={vocabColors}
						placeholder="Type a label and press space"
						on:change={(e) => (pasteLabels = e.detail)}
					/>
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
