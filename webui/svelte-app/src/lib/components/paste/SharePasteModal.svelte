<script lang="ts">
	import Modal from '$lib/components/ui/Modal.svelte';
	import Button from '$lib/components/ui/Button.svelte';
	import Input from '$lib/components/ui/Input.svelte';
	import { toast } from '$lib/stores/toast';
	import { listPasteShares, sharePaste, unsharePaste, transferPaste } from '$lib/api/pastes';
	import { formatApiError } from '$lib/utils/errors';
	import type { PasteShare } from '$lib/types/paste';

	export let pasteId: string | null = null;
	export let pasteName: string = '';
	export let token: string;
	export let isOwner: boolean = true;
	/** Set for team pastes: those are shared with the whole team, so only
	 * the ownership transfer applies. */
	export let team: string = '';
	export let onClose: () => void;

	let shares: PasteShare[] = [];
	let loading = true;
	let error = '';
	let shareUsername = '';
	let sharingInProgress = false;
	let transferUsername = '';
	let transferInProgress = false;
	let confirmingTransfer = false;
	let unshareTarget: string | null = null;
	let unshareInProgress = false;

	$: if (pasteId) {
		confirmingTransfer = false;
		unshareTarget = null;
		loadShares();
	}

	async function loadShares() {
		if (!pasteId) return;
		if (team) {
			shares = [];
			loading = false;
			return;
		}

		loading = true;
		error = '';

		try {
			shares = await listPasteShares(pasteId, token);
		} catch (err) {
			error = formatApiError(err);
		} finally {
			loading = false;
		}
	}

	async function handleShare() {
		if (!pasteId || !shareUsername.trim()) return;

		sharingInProgress = true;
		error = '';

		try {
			await sharePaste(pasteId, shareUsername.trim(), token);
			shareUsername = '';
			toast.show('Paste shared successfully', 'success');
			await loadShares();
		} catch (err) {
			error = formatApiError(err);
			toast.show(error, 'error', 5000);
		} finally {
			sharingInProgress = false;
		}
	}

	async function handleUnshare() {
		if (!pasteId || !unshareTarget) return;

		unshareInProgress = true;
		error = '';
		try {
			await unsharePaste(pasteId, unshareTarget, token);
			toast.show(`Share with ${unshareTarget} removed`, 'success');
			unshareTarget = null;
			await loadShares();
		} catch (err) {
			error = formatApiError(err);
		} finally {
			unshareInProgress = false;
		}
	}

	async function handleTransfer() {
		if (!pasteId || !transferUsername.trim()) return;

		if (!confirmingTransfer) {
			confirmingTransfer = true;
			return;
		}

		transferInProgress = true;
		error = '';

		try {
			await transferPaste(pasteId, transferUsername.trim(), token);
			toast.show('Ownership transferred', 'success');
			onClose();
		} catch (err) {
			error = formatApiError(err);
		} finally {
			transferInProgress = false;
			confirmingTransfer = false;
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
			{team ? 'Transfer' : 'Share'}: <span class="text-blue-600 dark:text-blue-500">{pasteName}</span>
		</h2>

		{#if error}
			<div class="p-3 bg-red-100 dark:bg-red-900 text-red-700 dark:text-red-200 rounded-md">
				{error}
			</div>
		{/if}

		{#if isOwner && team}
			<p class="text-sm text-gray-600 dark:text-gray-400">
				Team pastes are shared with every member of <strong>{team}</strong>; they cannot be shared
				with individual users.
			</p>
		{:else if isOwner}
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
										{#if unshareTarget === share.username}
											<div class="flex items-center justify-end gap-2">
												<span class="text-xs text-gray-600 dark:text-gray-400">
													{share.username} will lose access to this paste.
												</span>
												<Button
													on:click={() => (unshareTarget = null)}
													variant="secondary"
													disabled={unshareInProgress}
												>
													Cancel
												</Button>
												<Button on:click={handleUnshare} variant="danger" disabled={unshareInProgress}>
													Remove
												</Button>
											</div>
										{:else}
											<Button on:click={() => (unshareTarget = share.username)} variant="danger">
												Remove
											</Button>
										{/if}
									</td>
								</tr>
							{/each}
						</tbody>
					</table>
				</div>
			{/if}
		{/if}

		{#if isOwner}
			<div class="border-t border-gray-200 dark:border-gray-700 pt-4 space-y-2">
				<h3 class="text-sm font-semibold text-gray-900 dark:text-gray-100">Transfer ownership</h3>
				<p class="text-xs text-gray-600 dark:text-gray-400">
					{#if team}
						Hand this paste over to another member of {team} who can create pastes (not a viewer).
					{:else}
						Give another user full control of this paste.
					{/if}
				</p>
				<div class="flex gap-2">
					<Input
						bind:value={transferUsername}
						placeholder="New owner username or email"
						disabled={transferInProgress}
						on:keydown={() => (confirmingTransfer = false)}
					/>
					{#if confirmingTransfer}
						<Button
							on:click={() => (confirmingTransfer = false)}
							variant="secondary"
							disabled={transferInProgress}
						>
							Cancel
						</Button>
						<Button on:click={handleTransfer} variant="danger" disabled={!transferUsername.trim() || transferInProgress}>
							{transferInProgress ? 'Transferring...' : 'Yes, transfer'}
						</Button>
					{:else}
						<Button
							on:click={handleTransfer}
							variant="danger"
							disabled={!transferUsername.trim() || transferInProgress}
						>
							Transfer
						</Button>
					{/if}
				</div>
				{#if confirmingTransfer}
					<div class="p-3 bg-amber-50 dark:bg-amber-900/30 border border-amber-200 dark:border-amber-800 rounded-md space-y-2 text-sm text-gray-700 dark:text-gray-300">
						<p class="font-medium text-gray-900 dark:text-gray-100">
							Transfer "{pasteName}" to <strong>{transferUsername.trim()}</strong>?
						</p>
						<ul class="list-disc list-inside space-y-1 text-gray-600 dark:text-gray-400">
							{#if team}
								<li>They become the owner of this team paste and can delete or transfer it.</li>
								<li>Unless you own the team, you lose owner control; you keep access as a team member.</li>
							{:else}
								<li>The other user becomes the owner and gains full control (view, delete, change privacy, re-share, transfer again).</li>
								<li>You will lose owner control; the paste will appear under "Shared with me" only if the new owner shares it back.</li>
								<li>Your personal labels are removed from it; existing shares with other users remain in place.</li>
							{/if}
						</ul>
					</div>
				{/if}
			</div>
		{:else}
			<p class="text-gray-600 dark:text-gray-400">
				Only the owner of this paste can manage sharing and ownership.
			</p>
		{/if}

		<div class="flex justify-end">
			<Button on:click={onClose} variant="secondary">Close</Button>
		</div>
	</div>
</Modal>
