<script lang="ts">
	import { goto } from '$app/navigation';
	import { onMount } from 'svelte';
	import { auth } from '$lib/stores/auth';
	import { listPastes, searchPastes, deletePaste, updatePaste } from '$lib/api/pastes';
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
	import type { Paste } from '$lib/types/paste';
	import {
		Eye,
		EyeOff,
		Link,
		Share2,
		Globe,
		Lock,
		Trash2,
		Search,
		X
	} from 'lucide-svelte';

	let pastes: Paste[] = [];
	let loading = true;
	let error = '';
	let page = 1;
	let totalPages = 1;
	let maxResults = 20;
	let deletingPaste: Paste | null = null;
	let sharingPaste: { id: string; name: string } | null = null;
	let copyTooltip: string | null = null;
	let searchQuery = '';
	let isSearching = false;

	async function loadPastes() {
		if (!$auth.token) {
			const currentPath = encodeURIComponent(window.location.pathname);
			goto(`/login?next=${currentPath}`);
			return;
		}

		loading = true;
		error = '';

		try {
			let response;
			if (isSearching && searchQuery.trim()) {
				response = await searchPastes(searchQuery.trim(), page, maxResults, $auth.token);
			} else {
				response = await listPastes(page, maxResults, $auth.token);
			}
			pastes = response.pastes || [];
			totalPages = response.total_pages;
		} catch (err) {
			error = formatApiError(err);
		} finally {
			loading = false;
		}
	}

	onMount(loadPastes);

	function handlePageChange(newPage: number) {
		if (newPage < 1 || newPage > totalPages) return;
		page = newPage;
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
		sharingPaste = { id: paste.paste_id, name: paste.name };
	}

	async function copyPasteUrl(paste: Paste, event: Event) {
		event.stopPropagation();
		const url = paste.public
			? `${window.location.origin}/public/p/${paste.paste_id}`
			: `${window.location.origin}/p/${paste.paste_id}`;

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
									<span class="text-xs text-gray-500 dark:text-gray-400 px-2 py-0.5 bg-gray-100 dark:bg-gray-700 rounded">
										{paste.language}
									</span>
								</div>
							</div>
							<p class="text-xs text-gray-600 dark:text-gray-400 mt-0.5">
								{timeAgo(paste.created_at)}
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

							<IconButton title="Share paste" on:click={(e) => initShare(paste, e)}>
								<Share2 class="w-4 h-4" />
							</IconButton>

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

							<IconButton
								title="Delete paste"
								variant="danger"
								on:click={(e) => handleDelete(paste, e)}
							>
								<Trash2 class="w-4 h-4" />
							</IconButton>
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
		<h2 class="text-xl font-bold text-gray-900 dark:text-gray-100">Confirm Delete</h2>
		<p class="text-gray-700 dark:text-gray-300">
			Are you sure you want to delete <strong>{deletingPaste?.name}</strong>?
		</p>
		<div class="flex justify-end gap-2">
			<Button on:click={() => (deletingPaste = null)} variant="secondary">Cancel</Button>
			<Button on:click={confirmDelete} variant="danger">Delete</Button>
		</div>
	</div>
</Modal>

<!-- Share Modal -->
{#if sharingPaste && $auth.token}
	<SharePasteModal
		pasteId={sharingPaste.id}
		pasteName={sharingPaste.name}
		token={$auth.token}
		onClose={() => (sharingPaste = null)}
	/>
{/if}
