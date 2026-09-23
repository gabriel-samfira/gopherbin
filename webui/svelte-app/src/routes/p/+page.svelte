<script lang="ts">
	import { goto } from '$app/navigation';
	import { page as appPage } from '$app/stores';
	import { onMount } from 'svelte';
	import { auth } from '$lib/stores/auth';
	import { listPastes, searchPastes, deletePaste, updatePaste, getLabelVocabulary, setPasteLabels } from '$lib/api/pastes';
	import LabelInput from '$lib/components/ui/LabelInput.svelte';
	import Button from '$lib/components/ui/Button.svelte';
	import IconButton from '$lib/components/ui/IconButton.svelte';
	import Input from '$lib/components/ui/Input.svelte';
	import Spinner from '$lib/components/ui/Spinner.svelte';
	import Modal from '$lib/components/ui/Modal.svelte';
	import SharePasteModal from '$lib/components/paste/SharePasteModal.svelte';
	import PastePreview from '$lib/components/paste/PastePreview.svelte';
	import { timeAgo } from '$lib/utils/date';
	import { copyToClipboard } from '$lib/utils/clipboard';
	import { formatApiError } from '$lib/utils/errors';
	import { decodeBase64 } from '$lib/utils/base64';
	import { toast } from '$lib/stores/toast';
	import { labelColor, labelStyle } from '$lib/utils/labelColor';
	import type { Paste, PasteScope } from '$lib/types/paste';
	import {
		Eye,
		EyeOff,
		Link,
		Share2,
		Globe,
		Lock,
		Trash2,
		Search,
		Tag,
		Users,
		X
	} from 'lucide-svelte';

	let pastes: Paste[] = [];
	let loading = true;
	let error = '';
	let page = 1;
	let totalPages = 1;
	let maxResults = 20;
	let deletingPaste: Paste | null = null;
	let sharingPaste: { id: string; name: string; owner: boolean } | null = null;
	let copyTooltip: string | null = null;
	let searchQuery = '';
	let isSearching = false;
	let scope: PasteScope = 'all';
	let labelFilters: string[] = [];
	let teamFilter = '';
	let personalLabels: string[] = [];
	let vocabTeams: { team: string; labels: string[] }[] = [];
	let vocabColors: Record<string, string> = {};
	let editingLabels: Paste | null = null;
	let editingLabelList: string[] = [];
	let savingLabels = false;

	$: allSuggestions = [...new Set([...personalLabels, ...vocabTeams.flatMap((t) => t.labels)])];

	async function loadVocabulary() {
		if (!$auth.token) return;
		try {
			const vocab = await getLabelVocabulary($auth.token);
			personalLabels = vocab.personal || [];
			vocabTeams = vocab.teams || [];
			vocabColors = vocab.colors || {};
		} catch {
			// Vocabulary is best-effort; filtering still works by typing.
		}
	}

	// The JWT 'user' claim holds the numeric user ID
	function isOwner(paste: Paste): boolean {
		return !!$auth.username && paste.owner_id === Number($auth.username);
	}

	async function loadPastes() {
		if (!$auth.token) {
			const currentPath = encodeURIComponent($appPage.url.pathname);
			goto(`/login?next=${currentPath}`);
			return;
		}

		loading = true;
		error = '';

		try {
			let response;
			if (isSearching && searchQuery.trim()) {
				response = await searchPastes(searchQuery.trim(), page, maxResults, $auth.token, scope, labelFilters, teamFilter);
			} else {
				response = await listPastes(page, maxResults, $auth.token, scope, labelFilters, teamFilter);
			}
			pastes = response.pastes || [];
			totalPages = response.total_pages;
		} catch (err) {
			error = formatApiError(err);
		} finally {
			loading = false;
		}
	}

	onMount(() => {
		loadPastes();
		loadVocabulary();
	});

	function handlePageChange(newPage: number) {
		if (newPage < 1 || newPage > totalPages) return;
		page = newPage;
		loadPastes();
	}

	function handleScopeChange(newScope: string) {
		if (scope === newScope) return;
		scope = newScope as PasteScope;
		page = 1;
		loadPastes();
	}

	function handleSearch() {
		if (searchQuery.trim()) {
			isSearching = true;
			page = 1;
			loadPastes();
		}
	}

	function clearSearch() {
		searchQuery = '';
		isSearching = false;
		page = 1;
		loadPastes();
	}

	function handleFilterLabelsChange(e: CustomEvent<string[]>) {
		labelFilters = e.detail;
		page = 1;
		loadPastes();
	}

	function handleTeamFilterChange() {
		page = 1;
		loadPastes();
	}

	function initEditLabels(paste: Paste, event: Event) {
		event.stopPropagation();
		editingLabels = paste;
		editingLabelList = (paste.labels || []).map((l) => l.name);
	}

	async function confirmEditLabels() {
		if (!editingLabels || !$auth.token || savingLabels) return;
		savingLabels = true;
		try {
			await setPasteLabels(editingLabels.paste_id, editingLabelList, $auth.token);
			editingLabels = null;
			toast.show('Labels updated', 'success');
			await loadPastes();
			await loadVocabulary();
		} catch (err) {
			error = formatApiError(err);
		} finally {
			savingLabels = false;
		}
	}

	function handleSearchKeyDown(e: KeyboardEvent) {
		if (e.key === 'Enter') {
			handleSearch();
		}
	}

	function viewPaste(paste: Paste) {
		const url = paste.public ? `/public/p/${paste.paste_id}` : `/p/${paste.paste_id}`;
		goto(url);
	}

	function initDelete(paste: Paste) {
		deletingPaste = paste;
	}

	async function confirmDelete() {
		if (!deletingPaste || !$auth.token) return;

		try {
			await deletePaste(deletingPaste.paste_id, $auth.token);
			deletingPaste = null;
			await loadPastes();
		} catch (err) {
			error = formatApiError(err);
		}
	}

	async function togglePrivacy(paste: Paste, event: Event) {
		event.stopPropagation();
		if (!$auth.token) return;

		try {
			await updatePaste(paste.paste_id, { public: !paste.public }, $auth.token);
			loadPastes();
		} catch (err) {
			error = formatApiError(err);
		}
	}

	function initShare(paste: Paste, event: Event) {
		event.stopPropagation();
		sharingPaste = { id: paste.paste_id, name: paste.name, owner: isOwner(paste) };
	}

	async function copyPasteUrl(paste: Paste, event: Event) {
		event.stopPropagation();
		const url = paste.public
			? `${$appPage.url.origin}/public/p/${paste.paste_id}`
			: `${$appPage.url.origin}/p/${paste.paste_id}`;

		try {
			await copyToClipboard(url);
			copyTooltip = paste.paste_id;
			setTimeout(() => {
				copyTooltip = null;
			}, 2000);
		} catch (err) {
			error = 'Failed to copy URL to clipboard';
		}
	}

	function handleDelete(paste: Paste, event: Event) {
		event.stopPropagation();
		initDelete(paste);
	}
</script>

<svelte:head>
	<title>My Pastes - GopherBin</title>
</svelte:head>

<div class="space-y-6">
	<!-- Header with title and search -->
	<div class="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4">
		<h1 class="text-2xl sm:text-3xl font-bold text-gray-900 dark:text-gray-100">My Pastes</h1>

		<div class="flex gap-2 w-full sm:flex-1 sm:max-w-md">
			<div class="relative flex-1">
				<Input
					bind:value={searchQuery}
					placeholder="Search by name or content..."
					on:keydown={handleSearchKeyDown}
				/>
				{#if isSearching}
					<button
						on:click={clearSearch}
						class="absolute right-2 top-1/2 -translate-y-1/2 text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200"
						title="Clear search"
					>
						<X class="w-4 h-4" />
					</button>
				{/if}
			</div>
			<Button on:click={handleSearch} variant="primary" disabled={!searchQuery.trim()}>
				<Search class="w-4 h-4" />
			</Button>
		</div>
	</div>

	<!-- Scope tabs -->
	<div class="flex gap-2">
		{#each [{ id: 'all', label: 'All' }, { id: 'mine', label: 'Mine' }, { id: 'shared', label: 'Shared with me' }] as tab}
			<button
				type="button"
				on:click={() => handleScopeChange(tab.id)}
				class="px-3 py-1.5 rounded-md text-sm font-medium transition-colors
					{scope === tab.id
					? 'bg-blue-600 text-white'
					: 'bg-gray-100 dark:bg-gray-700 text-gray-700 dark:text-gray-300 hover:bg-gray-200 dark:hover:bg-gray-600'}"
			>
				{tab.label}
			</button>
		{/each}
	</div>

	<!-- Label & team filters -->
	<div class="flex flex-col sm:flex-row gap-3 sm:items-center">
		<div class="flex-1">
			<LabelInput
				labels={labelFilters}
				suggestions={allSuggestions}
				colorMap={vocabColors}
				placeholder="Filter by label — type and press space"
				on:change={handleFilterLabelsChange}
			/>
		</div>
		<select
			bind:value={teamFilter}
			on:change={handleTeamFilterChange}
			class="px-3 h-10 border rounded-md bg-white dark:bg-gray-800 text-gray-900 dark:text-gray-100 border-gray-300 dark:border-gray-600 text-sm"
			title="Filter by team"
		>
			<option value="">All teams</option>
			{#each vocabTeams as t}
				<option value={t.team}>{t.team}</option>
			{/each}
		</select>
	</div>

	{#if error}
		<div class="p-3 bg-red-100 dark:bg-red-900 text-red-700 dark:text-red-200 rounded-md">
			{error}
		</div>
	{/if}

	{#if loading}
		<Spinner />
	{:else if pastes.length === 0}
		<div class="text-center py-12">
			{#if isSearching}
				<p class="text-gray-600 dark:text-gray-400 mb-4">
					No pastes found matching "{searchQuery}"
				</p>
				<Button on:click={clearSearch} variant="secondary">Clear search</Button>
			{:else if scope === 'shared'}
				<p class="text-gray-600 dark:text-gray-400 mb-4">Nothing has been shared with you yet.</p>
			{:else if scope === 'mine'}
				<p class="text-gray-600 dark:text-gray-400 mb-4">You haven't created any pastes yet.</p>
				<Button on:click={() => goto('/')} variant="primary">Create new paste</Button>
			{:else}
				<p class="text-gray-600 dark:text-gray-400 mb-4">You haven't created any pastes yet.</p>
				<Button on:click={() => goto('/')} variant="primary">Create new paste</Button>
			{/if}
		</div>
	{:else}
		{#if totalPages > 1}
			<div class="flex justify-center gap-4">
				<Button
					on:click={() => handlePageChange(page - 1)}
					disabled={page === 1}
					variant="secondary"
				>
					Newer
				</Button>
				<span class="py-2 text-gray-700 dark:text-gray-300">
					Page {page} of {totalPages}
				</span>
				<Button
					on:click={() => handlePageChange(page + 1)}
					disabled={page === totalPages}
					variant="secondary"
				>
					Older
				</Button>
			</div>
		{/if}

		<div class="space-y-3">
			{#each pastes as paste}
				<div
					class="bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-700 rounded-lg overflow-hidden hover:shadow-md transition-shadow"
				>
					<!-- Header -->
					<div class="p-3 flex flex-col sm:flex-row sm:items-center sm:justify-between gap-3">
						<!-- Left: Title and metadata -->
						<div class="flex-1 min-w-0">
							<div class="flex items-center gap-2 flex-wrap">
								<h3
									class="text-base font-semibold text-gray-900 dark:text-gray-100 truncate"
								>
									{paste.name}
								</h3>
								<div class="flex items-center gap-2">
									{#if paste.public}
										<Globe class="w-4 h-4 text-green-600 dark:text-green-500 flex-shrink-0" />
									{:else}
										<Lock class="w-4 h-4 text-gray-500 dark:text-gray-400 flex-shrink-0" />
									{/if}
									{#if paste.team}
										<span
											class="flex items-center gap-1 text-xs text-purple-700 dark:text-purple-300 px-2 py-0.5 bg-purple-100 dark:bg-purple-900 rounded"
											title="Team paste"
										>
											<Users class="w-3 h-3" />
											{paste.team}
										</span>
									{/if}
									<span class="text-xs text-gray-500 dark:text-gray-400 px-2 py-0.5 bg-gray-100 dark:bg-gray-700 rounded">
										{paste.language}
									</span>
									{#each paste.labels || [] as label}
										<span
											class="text-xs px-2 py-0.5 rounded-full {labelColor(label.name, label.color)} {label.scope === 'team' ? 'ring-1 ring-purple-400 dark:ring-purple-500' : ''}"
															style={labelStyle(label.color)}
											title={label.scope === 'team' ? `Team label · ${label.team}` : 'Personal label'}
										>
											{label.name}
										</span>
									{/each}
								</div>
							</div>
							<p class="text-xs text-gray-600 dark:text-gray-400 mt-0.5">
								{timeAgo(paste.created_at)}
								{#if paste.owner && !isOwner(paste)}
									· owned by {paste.owner}
								{/if}
							</p>
						</div>

						<!-- Right: Actions -->
						<div class="flex items-center gap-1 flex-shrink-0">
							<div class="relative">
								<IconButton title="Copy URL" on:click={(e) => copyPasteUrl(paste, e)}>
									<Link class="w-4 h-4" />
								</IconButton>
								{#if copyTooltip === paste.paste_id}
									<div
										class="absolute top-full mt-1 right-0 px-2 py-1 bg-gray-800 dark:bg-gray-700 text-white text-xs rounded-md whitespace-nowrap z-10"
									>
										Copied!
									</div>
								{/if}
							</div>

							{#if isOwner(paste)}
								<IconButton title="Edit labels" on:click={(e) => initEditLabels(paste, e)}>
									<Tag class="w-4 h-4" />
								</IconButton>

								<IconButton title="Share paste" on:click={(e) => initShare(paste, e)}>
									<Share2 class="w-4 h-4" />
								</IconButton>

								{#if !paste.team}
									<IconButton
										title={paste.public ? 'Make private' : 'Make public'}
										on:click={(e) => togglePrivacy(paste, e)}
									>
										{#if paste.public}
											<EyeOff class="w-4 h-4" />
										{:else}
											<Eye class="w-4 h-4" />
										{/if}
									</IconButton>
								{/if}

								<IconButton
									title="Delete paste"
									variant="danger"
									on:click={(e) => handleDelete(paste, e)}
								>
									<Trash2 class="w-4 h-4" />
								</IconButton>
							{/if}
						</div>
					</div>

					<!-- Preview (always visible, clickable) -->
					{#if paste.preview}
						<div
							class="border-t border-gray-200 dark:border-gray-700 cursor-pointer hover:opacity-80 transition-opacity"
							on:click={() => viewPaste(paste)}
							on:keydown={(e) => e.key === 'Enter' && viewPaste(paste)}
							role="button"
							tabindex="0"
						>
							<PastePreview content={decodeBase64(paste.preview)} language={paste.language} />
						</div>
					{/if}
				</div>
			{/each}
		</div>

		{#if totalPages > 1}
			<div class="flex justify-center gap-4">
				<Button
					on:click={() => handlePageChange(page - 1)}
					disabled={page === 1}
					variant="secondary"
				>
					Newer
				</Button>
				<span class="py-2 text-gray-700 dark:text-gray-300">
					Page {page} of {totalPages}
				</span>
				<Button
					on:click={() => handlePageChange(page + 1)}
					disabled={page === totalPages}
					variant="secondary"
				>
					Older
				</Button>
			</div>
		{/if}
	{/if}
</div>

<!-- Delete Modal -->
<Modal show={!!deletingPaste} onClose={() => (deletingPaste = null)}>
	<div class="space-y-4">
		<h2 class="text-xl font-bold text-gray-900 dark:text-gray-100">
			Delete paste <strong class="text-red-600 dark:text-red-500">{deletingPaste?.name}</strong>?
		</h2>
		<div class="p-3 bg-red-50 dark:bg-red-900/30 border border-red-200 dark:border-red-800 rounded-md space-y-2 text-sm text-gray-700 dark:text-gray-300">
			<p class="font-medium text-gray-900 dark:text-gray-100">Deleting this paste will:</p>
			<ul class="list-disc list-inside space-y-1 text-gray-600 dark:text-gray-400">
				<li>permanently remove its content and its share URL, which will stop working immediately;</li>
				{#if deletingPaste?.team}
					<li>remove it from team <strong>{deletingPaste.team}</strong>, so all members lose access;</li>
				{:else}
					<li>revoke access for every user it was shared with;</li>
				{/if}
				<li>this operation cannot be undone.</li>
			</ul>
		</div>
		<div class="flex justify-end gap-2">
			<Button on:click={() => (deletingPaste = null)} variant="secondary">Cancel</Button>
			<Button on:click={confirmDelete} variant="danger">Delete</Button>
		</div>
	</div>
</Modal>

<!-- Labels Modal -->
<Modal show={!!editingLabels} onClose={() => (editingLabels = null)}>
	<div class="space-y-4">
		<h2 class="text-xl font-bold text-gray-900 dark:text-gray-100">
			Labels for <strong>{editingLabels?.name}</strong>
		</h2>
		<p class="text-sm text-gray-600 dark:text-gray-400">
			{#if editingLabels?.team}
				Team paste: labels are shared vocabulary of <strong>{editingLabels.team}</strong>.
			{:else}
				Personal labels are only visible to you.
			{/if}
			Press space to add a label.
		</p>
		<LabelInput
			labels={editingLabelList}
			suggestions={allSuggestions}
			colorMap={vocabColors}
			disabled={savingLabels}
			on:change={(e) => (editingLabelList = e.detail)}
		/>
		<div class="flex justify-end gap-2">
			<Button on:click={() => (editingLabels = null)} variant="secondary">Cancel</Button>
			<Button on:click={confirmEditLabels} disabled={savingLabels}>
				{savingLabels ? 'Saving...' : 'Save labels'}
			</Button>
		</div>
	</div>
</Modal>

<!-- Share Modal -->
{#if sharingPaste && $auth.token}
	<SharePasteModal
		pasteId={sharingPaste.id}
		pasteName={sharingPaste.name}
		isOwner={sharingPaste.owner}
		token={$auth.token}
		onClose={() => {
			sharingPaste = null;
			loadPastes();
		}}
	/>
{/if}
