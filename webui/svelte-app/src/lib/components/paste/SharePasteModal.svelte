<script lang="ts">
	import { onMount } from 'svelte';
	import Modal from '$lib/components/ui/Modal.svelte';
	import Button from '$lib/components/ui/Button.svelte';
	import Input from '$lib/components/ui/Input.svelte';
	import { toast } from '$lib/stores/toast';
	import { listPasteShares, sharePaste, unsharePaste } from '$lib/api/pastes';
	import type { PasteShare } from '$lib/types/paste';
	import type { ApiError } from '$lib/types/api';

	export let pasteId: string | null = null;
	export let pasteName: string = '';
	export let token: string;
	export let onClose: () => void;

	let shares: PasteShare[] = [];
	let loading = true;
	let error = '';
	let shareUsername = '';
	let sharingInProgress = false;

	$: if (pasteId) {
		console.log('SharePasteModal pasteId:', pasteId); // Debug log
		loadShares();
	}

	async function loadShares() {
		if (!pasteId) return;

		loading = true;
		error = '';

		try {
			shares = await listPasteShares(pasteId, token);
		} catch (err) {
			const apiError = err as ApiError;
			error = apiError.details || apiError.error || 'Failed to load shares';
		} finally {
			loading = false;
		}
	}

	async function handleShare() {
		if (!pasteId || !shareUsername.trim()) return;

		console.log('Sharing paste:', pasteId, 'with user:', shareUsername.trim()); // Debug log

		sharingInProgress = true;
		error = '';

		try {
			await sharePaste(pasteId, shareUsername.trim(), token);
			shareUsername = '';
			toast.show('Paste shared successfully', 'success');
			await loadShares();
		} catch (err) {
			const apiError = err as ApiError;
			error = apiError.details || apiError.error || 'Failed to share paste';
			toast.show(error, 'error', 5000);
		} finally {
			sharingInProgress = false;
		}
	}

	async function handleUnshare(username: string) {
		if (!pasteId) return;

		try {
			await unsharePaste(pasteId, username, token);
			await loadShares();
		} catch (err) {
			const apiError = err as ApiError;
			error = apiError.details || apiError.error || 'Failed to remove share';
		}
	}

	function handleKeyDown(e: KeyboardEvent) {
		if (e.key === 'Enter') {
			handleShare();
		}
	}
</script>

<Modal show={!!pasteId} onClose={onClose}>
	<div class="space-y-4">
		<h2 class="text-xl font-bold text-gray-900 dark:text-gray-100">
			Share: <span class="text-blue-600 dark:text-blue-500">{pasteName}</span>
		</h2>

		{#if error}
			<div class="p-3 bg-red-100 dark:bg-red-900 text-red-700 dark:text-red-200 rounded-md">
				{error}
			</div>
		{/if}

		<div class="flex gap-2">
			<Input
				bind:value={shareUsername}
				placeholder="Username or email"
				on:keydown={handleKeyDown}
				disabled={sharingInProgress}
			/>
			<Button
				on:click={handleShare}
				variant="success"
				disabled={!shareUsername.trim() || sharingInProgress}
			>
				Share
			</Button>
		</div>

		{#if loading}
			<div class="text-center py-4">
				<div
					class="animate-spin rounded-full h-8 w-8 border-b-2 border-blue-600 dark:border-blue-500 mx-auto"
				></div>
			</div>
		{:else if shares.length === 0}
			<p class="text-gray-600 dark:text-gray-400 text-center py-4">
				No shares yet. Add a username or email above to share this paste.
			</p>
		{:else}
			<div class="border border-gray-200 dark:border-gray-700 rounded-lg overflow-hidden">
				<table class="min-w-full divide-y divide-gray-200 dark:divide-gray-700">
					<thead class="bg-gray-50 dark:bg-gray-900">
						<tr>
							<th
								class="px-4 py-2 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase"
							>
								Full Name
							</th>
							<th
								class="px-4 py-2 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase"
							>
								Username
							</th>
							<th
								class="px-4 py-2 text-right text-xs font-medium text-gray-500 dark:text-gray-400 uppercase"
							>
								Action
							</th>
						</tr>
					</thead>
					<tbody class="divide-y divide-gray-200 dark:divide-gray-700">
						{#each shares as share}
							<tr class="hover:bg-gray-50 dark:hover:bg-gray-700">
								<td class="px-4 py-2 text-sm text-gray-900 dark:text-gray-100">
									{share.full_name}
								</td>
								<td class="px-4 py-2 text-sm text-gray-900 dark:text-gray-100">
									{share.username}
								</td>
								<td class="px-4 py-2 text-sm text-right">
									<Button on:click={() => handleUnshare(share.username)} variant="danger">
										Remove
									</Button>
								</td>
							</tr>
						{/each}
					</tbody>
				</table>
			</div>
		{/if}

		<div class="flex justify-end">
			<Button on:click={onClose} variant="secondary">Close</Button>
		</div>
	</div>
</Modal>
