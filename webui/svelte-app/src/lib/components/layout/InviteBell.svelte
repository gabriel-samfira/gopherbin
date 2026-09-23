<script lang="ts">
	import { onDestroy } from 'svelte';
	import { goto } from '$app/navigation';
	import { auth } from '$lib/stores/auth';
	import {
		listPendingInvites,
		acceptTeamInvite,
		declineTeamInvite,
		listPendingTransfers,
		teamTransferAction
	} from '$lib/api/teams';
	import type { TeamInviteInfo, TeamTransferInfo } from '$lib/types/team';
	import { toast } from '$lib/stores/toast';
	import { noticesTick } from '$lib/stores/notices';
	import { formatApiError } from '$lib/utils/errors';
	import { Bell } from 'lucide-svelte';

	let invites: TeamInviteInfo[] = [];
	let transfers: TeamTransferInfo[] = [];
	let open = false;
	let busy = false;
	let declineTarget: string | null = null;

	$: total = invites.length + transfers.length;

	let refreshing = false;
	let refetchQueued = false;

	async function refresh(_tick?: number) {
		if (refreshing) {
			refetchQueued = true;
			return;
		}
		refreshing = true;
		try {
			if (!$auth.token) {
				invites = [];
				transfers = [];
			} else {
				[invites, transfers] = await Promise.all([
					listPendingInvites($auth.token),
					listPendingTransfers($auth.token)
				]);
			}
		} catch {
			// notice is best-effort; the teams page remains the source of truth
		} finally {
			refreshing = false;
			if (refetchQueued) {
				refetchQueued = false;
				refresh();
			}
		}
	}

	// Refresh on login/token change and whenever an invite/transfer is
	// accepted or declined anywhere in the app (see $lib/stores/notices).
	$: if ($auth.token) refresh($noticesTick);

	const pollTimer = setInterval(refresh, 60000);
	onDestroy(() => clearInterval(pollTimer));

	async function accept(invite: TeamInviteInfo) {
		if (!$auth.token || busy) return;
		busy = true;
		try {
			await acceptTeamInvite(invite.team_name, $auth.token);
			toast.show(`Joined ${invite.team_name}`, 'success');
			await refresh();
			open = false;
		} catch (err) {
			toast.error(formatApiError(err));
		} finally {
			busy = false;
		}
	}

	async function decline(invite: TeamInviteInfo) {
		if (!$auth.token || busy) return;
		busy = true;
		declineTarget = null;
		try {
			await declineTeamInvite(invite.team_name, $auth.token);
			toast.show(`Invitation to ${invite.team_name} declined`, 'success');
			await refresh();
		} catch (err) {
			toast.error(formatApiError(err));
		} finally {
			busy = false;
		}
	}

	async function acceptTransfer(transfer: TeamTransferInfo) {
		if (!$auth.token || busy) return;
		busy = true;
		try {
			await teamTransferAction(transfer.team_name, 'accept', $auth.token);
			toast.show(`You are now the owner of ${transfer.team_name}`, 'success');
			await refresh();
			open = false;
		} catch (err) {
			toast.error(formatApiError(err));
		} finally {
			busy = false;
		}
	}

	async function declineTransfer(transfer: TeamTransferInfo) {
		if (!$auth.token || busy) return;
		busy = true;
		declineTarget = null;
		try {
			await teamTransferAction(transfer.team_name, 'decline', $auth.token);
			toast.show(`Transfer of ${transfer.team_name} declined`, 'success');
			await refresh();
		} catch (err) {
			toast.error(formatApiError(err));
		} finally {
			busy = false;
		}
	}

	// Opening the notice list always re-fetches, so entries that are no
	// longer actionable (invite revoked, team deleted, offer cancelled by
	// the owner meanwhile) drop out without any polling.
	function toggleOpen() {
		open = !open;
		if (open) refresh();
	}

	function goTeams() {
		open = false;
		goto('/teams');
	}
</script>

{#if $auth.isAuthenticated && total > 0}
	<div class="relative">
		<button
			type="button"
			class="relative p-2 rounded-md text-gray-700 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700"
			aria-label="Team invitations and transfers ({total})"
			on:click={toggleOpen}
		>
			<Bell class="w-5 h-5" />
			<span
				class="absolute -top-0.5 -right-0.5 min-w-[18px] h-[18px] px-1 rounded-full bg-red-600 text-white text-[10px] font-bold flex items-center justify-center"
			>
				{total}
			</span>
		</button>
		{#if open}
			<div
				class="absolute right-0 mt-2 w-72 rounded-md border border-gray-200 dark:border-gray-700
					bg-white dark:bg-gray-800 shadow-lg z-50"
				role="menu"
			>
				{#if invites.length > 0}
				<div class="px-3 py-2 border-b border-gray-100 dark:border-gray-700 text-xs font-semibold
					text-gray-500 dark:text-gray-400 uppercase">
					Team invitations
				</div>
				{/if}
				<ul class="divide-y divide-gray-100 dark:divide-gray-700">
					{#each invites as invite (invite.team_id)}
						<li class="px-3 py-2">
							<p class="text-sm font-medium text-gray-900 dark:text-gray-100">{invite.team_name}</p>
							{#if invite.invited_by}
								<p class="text-xs text-gray-500 dark:text-gray-400">by {invite.invited_by}</p>
							{/if}
							<div class="flex items-center gap-3 mt-1.5">
								<button
									type="button"
									class="text-xs font-medium text-green-600 dark:text-green-400 hover:underline disabled:opacity-50"
									disabled={busy}
									on:click={() => accept(invite)}
								>Accept</button>
								{#if declineTarget === invite.team_name}
									<button
										type="button"
										class="text-xs font-medium text-red-600 dark:text-red-400 hover:underline disabled:opacity-50"
										disabled={busy}
										on:click={() => decline(invite)}
									>Confirm decline</button>
									<button
										type="button"
										class="text-xs text-gray-500 hover:underline"
										on:click={() => (declineTarget = null)}
									>Cancel</button>
								{:else}
									<button
										type="button"
										class="text-xs text-red-600 dark:text-red-400 hover:underline disabled:opacity-50"
										disabled={busy}
										on:click={() => (declineTarget = invite.team_name)}
									>Decline</button>
								{/if}
							</div>
						</li>
					{/each}
				</ul>
				{#if transfers.length > 0}
					<div class="px-3 py-2 border-y border-gray-100 dark:border-gray-700 text-xs font-semibold
						text-gray-500 dark:text-gray-400 uppercase">
						Ownership offers
					</div>
					<ul class="divide-y divide-gray-100 dark:divide-gray-700">
						{#each transfers as transfer (transfer.team_id)}
							<li class="px-3 py-2">
								<p class="text-sm font-medium text-gray-900 dark:text-gray-100">
									Take over {transfer.team_name}
								</p>
								{#if transfer.from_user}
									<p class="text-xs text-gray-500 dark:text-gray-400">from {transfer.from_user}</p>
								{/if}
								<div class="flex items-center gap-3 mt-1.5">
									<button
										type="button"
										class="text-xs font-medium text-green-600 dark:text-green-400 hover:underline disabled:opacity-50"
										disabled={busy}
										on:click={() => acceptTransfer(transfer)}
									>Accept</button>
									{#if declineTarget === `t:${transfer.team_id}`}
										<button
											type="button"
											class="text-xs font-medium text-red-600 dark:text-red-400 hover:underline disabled:opacity-50"
											disabled={busy}
											on:click={() => declineTransfer(transfer)}
										>Confirm decline</button>
										<button
											type="button"
											class="text-xs text-gray-500 hover:underline"
											on:click={() => (declineTarget = null)}
										>Cancel</button>
									{:else}
										<button
											type="button"
											class="text-xs text-red-600 dark:text-red-400 hover:underline disabled:opacity-50"
											disabled={busy}
											on:click={() => (declineTarget = `t:${transfer.team_id}`)}
										>Decline</button>
									{/if}
								</div>
							</li>
						{/each}
					</ul>
				{/if}
				<button
					type="button"
					class="w-full px-3 py-2 text-left text-xs text-blue-600 dark:text-blue-400 hover:bg-gray-50
						dark:hover:bg-gray-700 border-t border-gray-100 dark:border-gray-700"
					on:click={goTeams}
				>View all teams</button>
			</div>
		{/if}
	</div>
{/if}
