<script lang="ts">
	import { goto } from '$app/navigation';
	import { onMount } from 'svelte';
	import { auth } from '$lib/stores/auth';
	import { toast } from '$lib/stores/toast';
	import { listTeams, createTeam, acceptTeamInvite, declineTeamInvite } from '$lib/api/teams';
	import Button from '$lib/components/ui/Button.svelte';
	import Input from '$lib/components/ui/Input.svelte';
	import Modal from '$lib/components/ui/Modal.svelte';
	import Spinner from '$lib/components/ui/Spinner.svelte';
	import { formatApiError } from '$lib/utils/errors';
	import type { Team } from '$lib/types/team';
	import { Crown, MailQuestion, Users } from 'lucide-svelte';

	let teams: Team[] = [];
	let loading = true;
	let error = '';
	let page = 1;
	let totalPages = 1;
	const maxResults = 20;

	let showCreate = false;
	let newTeamName = '';
	let newTeamDescription = '';
	let creating = false;

	let decliningTeam: Team | null = null;
	let actionInProgress = false;

	$: invited = teams.filter((t) => t.my_role === 'pending');
	$: joined = teams.filter((t) => t.my_role !== 'pending');

	async function loadTeams() {
		if (!$auth.token) {
			goto('/login?next=/teams');
			return;
		}
		loading = true;
		error = '';
		try {
			const res = await listTeams(page, maxResults, $auth.token);
			teams = res.teams || [];
			totalPages = res.total_pages || 1;
		} catch (err) {
			error = formatApiError(err);
		} finally {
			loading = false;
		}
	}

	onMount(loadTeams);

	async function handleCreate(e: Event) {
		e.preventDefault();
		if (!$auth.token || !newTeamName.trim()) return;
		creating = true;
		error = '';
		try {
			const team = await createTeam(newTeamName.trim(), $auth.token, newTeamDescription.trim());
			newTeamName = '';
			newTeamDescription = '';
			showCreate = false;
			await loadTeams();
			goto(`/teams/${team.name}`);
		} catch (err) {
			error = formatApiError(err);
		} finally {
			creating = false;
		}
	}

	async function handleAccept(team: Team) {
		if (!$auth.token || actionInProgress) return;
		actionInProgress = true;
		try {
			await acceptTeamInvite(team.name, $auth.token);
			toast.show(`You joined ${team.name}`, 'success');
			await loadTeams();
		} catch (err) {
			error = formatApiError(err);
		} finally {
			actionInProgress = false;
		}
	}

	async function confirmDecline() {
		if (!$auth.token || !decliningTeam || actionInProgress) return;
		actionInProgress = true;
		try {
			await declineTeamInvite(decliningTeam.name, $auth.token);
			toast.show(`Invitation to ${decliningTeam.name} declined`, 'info');
			decliningTeam = null;
			await loadTeams();
		} catch (err) {
			error = formatApiError(err);
		} finally {
			actionInProgress = false;
		}
	}

	function handlePageChange(newPage: number) {
		if (newPage < 1 || newPage > totalPages) return;
		page = newPage;
		loadTeams();
	}
</script>

<svelte:head>
	<title>Teams - GopherBin</title>
</svelte:head>

<div class="space-y-6">
	<div class="flex items-center justify-between gap-4">
		<h1 class="text-2xl sm:text-3xl font-bold text-gray-900 dark:text-gray-100">Teams</h1>
		<Button on:click={() => (showCreate = true)} variant="primary">New team</Button>
	</div>

	{#if error}
		<div class="p-3 bg-red-100 dark:bg-red-900 text-red-700 dark:text-red-200 rounded-md">
			{error}
		</div>
	{/if}

	{#if loading}
		<Spinner />
	{:else if teams.length === 0}
		<div class="text-center py-12">
			<p class="text-gray-600 dark:text-gray-400 mb-4">You don't belong to any team yet.</p>
			<Button on:click={() => (showCreate = true)} variant="primary">Create a team</Button>
		</div>
	{:else}
		{#if invited.length > 0}
		<section class="space-y-3">
			<h2 class="text-lg font-semibold text-gray-900 dark:text-gray-100 flex items-center gap-2">
				<MailQuestion class="w-5 h-5 text-amber-600 dark:text-amber-400" />
				Invitations
				<span class="px-2 py-0.5 text-xs font-medium bg-amber-100 dark:bg-amber-900 text-amber-800 dark:text-amber-200 rounded-full">{invited.length}</span>
			</h2>
			<div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
				{#each invited as team}
					<div class="bg-white dark:bg-gray-800 border border-amber-300 dark:border-amber-700 rounded-lg p-4 space-y-3">
						<div class="flex items-center gap-2">
							<Users class="w-5 h-5 text-purple-600 dark:text-purple-400" />
							<button
								type="button"
								on:click={() => goto(`/teams/${team.name}`)}
								class="font-semibold text-gray-900 dark:text-gray-100 truncate hover:underline text-left"
							>
								{team.name}
							</button>
							<span class="ml-auto px-2 py-1 text-xs bg-amber-100 dark:bg-amber-900 text-amber-800 dark:text-amber-200 rounded">
								Invitation
							</span>
						</div>
						<p class="text-sm text-gray-600 dark:text-gray-400">
							{team.owner.full_name || team.owner.username} invited you to join this team.
						</p>
						<div class="flex justify-end gap-2">
							<Button on:click={() => (decliningTeam = team)} variant="secondary" disabled={actionInProgress}>
								Decline
							</Button>
							<Button on:click={() => handleAccept(team)} variant="success" disabled={actionInProgress}>
								Accept
							</Button>
						</div>
					</div>
				{/each}
			</div>
		</section>
		{/if}

		{#if joined.length > 0}
		<section class="space-y-3">
			<h2 class="text-lg font-semibold text-gray-900 dark:text-gray-100">Your teams</h2>
			{#if totalPages > 1}
				<div class="flex justify-center gap-4">
					<Button on:click={() => handlePageChange(page - 1)} disabled={page === 1} variant="secondary">
						Previous
					</Button>
					<span class="py-2 text-gray-700 dark:text-gray-300">Page {page} of {totalPages}</span>
					<Button
						on:click={() => handlePageChange(page + 1)}
						disabled={page === totalPages}
						variant="secondary"
					>
						Next
					</Button>
				</div>
			{/if}

			<div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
				{#each joined as team}
					<button
						type="button"
						on:click={() => goto(`/teams/${team.name}`)}
						class="text-left bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-700 rounded-lg p-4 hover:shadow-md transition-shadow space-y-2"
					>
						<div class="flex items-center gap-2">
							<Users class="w-5 h-5 text-purple-600 dark:text-purple-400" />
							<h3 class="font-semibold text-gray-900 dark:text-gray-100 truncate">{team.name}</h3>
							{#if team.my_role === 'owner'}
								<span class="ml-auto px-2 py-0.5 text-xs font-medium bg-purple-100 dark:bg-purple-900 text-purple-800 dark:text-purple-200 rounded">
									Owner
								</span>
							{:else}
								<span class="ml-auto px-2 py-0.5 text-xs font-medium bg-gray-100 dark:bg-gray-700 text-gray-700 dark:text-gray-300 rounded">
									Member
								</span>
							{/if}
						</div>
						{#if team.description}
							<p class="text-sm text-gray-600 dark:text-gray-400 line-clamp-2">{team.description}</p>
						{/if}
						<p class="flex items-center gap-1 text-xs text-gray-600 dark:text-gray-400">
							{#if team.my_role === 'owner'}
								<Crown class="w-3 h-3" />
								you own this team
							{:else}
								owned by {team.owner.full_name || team.owner.username}
							{/if}
						</p>
					</button>
				{/each}
			</div>
		</section>
		{/if}
	{/if}
</div>

<Modal show={showCreate} onClose={() => (showCreate = false)}>
	<form on:submit={handleCreate} class="space-y-4">
		<h2 class="text-xl font-bold text-gray-900 dark:text-gray-100">New team</h2>
		<Input
			bind:value={newTeamName}
			placeholder="Team name"
			disabled={creating}
		/>
		<textarea
			bind:value={newTeamDescription}
			rows="2"
			maxlength="254"
			placeholder="Short description (optional)"
			class="w-full px-3 py-2 border rounded-md bg-white dark:bg-gray-800 text-gray-900 dark:text-gray-100 border-gray-300 dark:border-gray-600 focus:outline-none focus:ring-2 focus:ring-blue-500"
		></textarea>
		<p class="text-xs text-gray-600 dark:text-gray-400">
			Team pastes are private and visible to all team members. Team members can create pastes
			for the team; only the team owner can manage members or delete the team. Invited users
			must accept before they get access.
		</p>
		<div class="flex justify-end gap-2">
			<Button on:click={() => (showCreate = false)} variant="secondary" type="button">Cancel</Button>
			<Button type="submit" variant="success" disabled={!newTeamName.trim() || creating}>
				Create
			</Button>
		</div>
	</form>
</Modal>

<Modal show={!!decliningTeam} onClose={() => (decliningTeam = null)}>
	<div class="space-y-4">
		<h2 class="text-xl font-bold text-gray-900 dark:text-gray-100">Decline invitation</h2>
		<div class="p-3 bg-amber-50 dark:bg-amber-900/30 border border-amber-200 dark:border-amber-800 rounded-md space-y-2 text-sm text-gray-700 dark:text-gray-300">
			<p class="font-medium text-gray-900 dark:text-gray-100">If you decline the invitation to <strong>{decliningTeam?.name}</strong>:</p>
			<ul class="list-disc list-inside space-y-1 text-gray-600 dark:text-gray-400">
				<li>You will not join the team and cannot see its pastes.</li>
				<li>The team owner will have to invite you again if you change your mind.</li>
			</ul>
		</div>
		<div class="flex justify-end gap-2">
			<Button on:click={() => (decliningTeam = null)} variant="secondary">Keep invitation</Button>
			<Button on:click={confirmDecline} variant="danger" disabled={actionInProgress}>Decline</Button>
		</div>
	</div>
</Modal>
