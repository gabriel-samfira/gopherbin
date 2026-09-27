<script lang="ts">
	import { page } from '$app/stores';
	import { goto } from '$app/navigation';
	import { onMount } from 'svelte';
	import { auth } from '$lib/stores/auth';
	import {
		getTeam,
		deleteTeam,
		listTeamMembers,
		addTeamMember,
		removeTeamMember,
		acceptTeamInvite,
		declineTeamInvite,
		leaveTeam,
		updateTeam,
		setTeamLabels,
		setTeamMemberRole,
		transferTeam,
		teamTransferAction
	} from '$lib/api/teams';
	import UserSearchInput from '$lib/components/ui/UserSearchInput.svelte';
	import LabelInput from '$lib/components/ui/LabelInput.svelte';
	import { updateLabel } from '$lib/api/pastes';
	import { labelColor, labelStyle } from '$lib/utils/labelColor';
	import Button from '$lib/components/ui/Button.svelte';
	import Input from '$lib/components/ui/Input.svelte';
	import Modal from '$lib/components/ui/Modal.svelte';
	import Spinner from '$lib/components/ui/Spinner.svelte';
	import { toast } from '$lib/stores/toast';
	import { formatApiError } from '$lib/utils/errors';
	import type { MemberRole, Team, TeamMember } from '$lib/types/team';
	import { ArrowLeftRight, Crown, LogOut, PaintBucket, RotateCcw, Trash2 } from 'lucide-svelte';

	let team: Team | null = null;
	let members: TeamMember[] = [];
	let loading = true;
	let error = '';

	let newMember = '';
	let inviteRole: MemberRole = 'member';
	let adding = false;

	let showTransfer = false;
	let transferTarget = '';
	let transferring = false;
	let transferBusy = false;

	let showEdit = false;
	let editName = '';
	let editDescription = '';
	let savingTeam = false;
	let teamLabels: string[] = [];
	let savingLabels = false;
	let labelsReqSeq = 0;
	let showDelete = false;
	let deleteConfirmation = '';
	let deleting = false;

	let removingMember: TeamMember | null = null;
	let showLeave = false;
	let leaving = false;
	let showDecline = false;
	let actionInProgress = false;

	$: teamName = $page.params.name ?? '';
	$: currentUserId = $auth.username ? Number($auth.username) : 0;
	$: isTeamOwner = !!team && team.my_role === 'owner';
	$: isInvited = !!team && team.my_role === 'pending';
	$: isMember = !!team && ['admin', 'member', 'viewer'].includes(team.my_role ?? '');
	$: canManageMembers = isTeamOwner || (!!team && team.my_role === 'admin');
	// Viewers are read-only; the backend refuses their label changes.
	$: canEditLabels = isTeamOwner || (!!team && ['admin', 'member'].includes(team.my_role ?? ''));
	$: myInviter = members.find((m) => m.id === currentUserId)?.added_by || '';
	$: transferPending = !!team && !!team.transfer_to;
	$: canDeleteTeam = isTeamOwner && deleteConfirmation === teamName;

	function canRemoveMember(member: TeamMember): boolean {
		if (!team || member.id === team.owner.id) return false;
		// Only the team owner may remove users allowed to remove others.
		return isTeamOwner || (canManageMembers && member.role !== 'admin');
	}

	function roleBadge(m: TeamMember): string {
		switch (m.role) {
			case 'admin':
				return 'bg-blue-100 dark:bg-blue-900 text-blue-800 dark:text-blue-200';
			case 'viewer':
				return 'bg-gray-100 dark:bg-gray-700 text-gray-700 dark:text-gray-300';
			default:
				return 'bg-green-100 dark:bg-green-900 text-green-800 dark:text-green-200';
		}
	}
	function roleLabel(m: TeamMember): string {
		return m.role === 'admin' ? 'Admin' : m.role === 'viewer' ? 'Viewer' : 'Member';
	}

	// The route param the page was last loaded for.
	let loadedFor = '';

	async function loadTeam() {
		if (!$auth.token) {
			const currentPath = encodeURIComponent($page.url.pathname);
			goto(`/login?next=${currentPath}`);
			return;
		}
		if (!teamName) return;
		loadedFor = teamName;

		loading = true;
		error = '';
		try {
			team = await getTeam(teamName, $auth.token);
			members = await listTeamMembers(teamName, $auth.token);
			teamLabels = team.labels || [];
		} catch (err) {
			error = formatApiError(err);
		} finally {
			loading = false;
		}
	}

	onMount(loadTeam);

	// SPA renames keep the component mounted; reload when the route param
	// changes. Compared with the param last loaded rather than team.name, which
	// may legitimately differ (e.g. case-insensitive name lookups on MySQL)
	// and would then reload forever.
	$: if (teamName && loadedFor && teamName !== loadedFor) loadTeam();

	async function handleAddMember(e: Event) {
		e.preventDefault();
		if (!$auth.token || !teamName || !newMember.trim()) return;
		adding = true;
		error = '';
		const invitee = newMember.trim();
		try {
			await addTeamMember(teamName, invitee, $auth.token, inviteRole);
			newMember = '';
			toast.show(`Invitation sent to ${invitee}`, 'success');
			await loadTeam();
		} catch (err) {
			error = formatApiError(err);
		} finally {
			adding = false;
		}
	}

	async function confirmRemoveMember() {
		if (!$auth.token || !teamName || !removingMember) return;
		actionInProgress = true;
		error = '';
		try {
			await removeTeamMember(teamName, removingMember.username, $auth.token);
			toast.show(`${removingMember.username} removed from team`, 'success');
			removingMember = null;
			await loadTeam();
		} catch (err) {
			error = formatApiError(err);
		} finally {
			actionInProgress = false;
		}
	}

	async function handleSetRole(member: TeamMember, role: string) {
		if (!$auth.token || !teamName) return;
		error = '';
		try {
			await setTeamMemberRole(teamName, member.username, role as MemberRole, $auth.token);
			toast.show(`${member.username} is now ${role}`, 'success');
			await loadTeam();
		} catch (err) {
			error = formatApiError(err);
			await loadTeam();
		}
	}

	async function confirmTransfer() {
		if (!$auth.token || !teamName || !transferTarget || transferring) return;
		transferring = true;
		error = '';
		try {
			await transferTeam(teamName, transferTarget, $auth.token);
			toast.show(`Ownership transfer offered to ${transferTarget}`, 'success');
			showTransfer = false;
			transferTarget = '';
			await loadTeam();
		} catch (err) {
			error = formatApiError(err);
		} finally {
			transferring = false;
		}
	}

	async function handleTransferAction(action: 'accept' | 'decline' | 'cancel') {
		if (!$auth.token || !teamName || transferBusy) return;
		transferBusy = true;
		error = '';
		try {
			await teamTransferAction(teamName, action, $auth.token);
			toast.show(
				action === 'accept'
					? `You are now the owner of ${teamName}`
					: action === 'decline'
						? 'Transfer declined'
						: 'Transfer cancelled',
				action === 'accept' ? 'success' : 'info'
			);
			await loadTeam();
		} catch (err) {
			error = formatApiError(err);
		} finally {
			transferBusy = false;
		}
	}

	async function handleAccept() {
		if (!$auth.token || !teamName || actionInProgress) return;
		actionInProgress = true;
		try {
			await acceptTeamInvite(teamName, $auth.token);
			toast.show(`You joined ${teamName}`, 'success');
			await loadTeam();
		} catch (err) {
			error = formatApiError(err);
		} finally {
			actionInProgress = false;
		}
	}

	async function confirmDecline() {
		if (!$auth.token || !teamName || actionInProgress) return;
		actionInProgress = true;
		try {
			await declineTeamInvite(teamName, $auth.token);
			toast.show('Invitation declined', 'info');
			showDecline = false;
			goto('/teams');
		} catch (err) {
			error = formatApiError(err);
		} finally {
			actionInProgress = false;
		}
	}

	async function confirmLeave() {
		if (!$auth.token || !teamName || leaving) return;
		leaving = true;
		try {
			await leaveTeam(teamName, $auth.token);
			toast.show(`You left ${teamName}`, 'success');
			goto('/teams');
		} catch (err) {
			error = formatApiError(err);
		} finally {
			leaving = false;
		}
	}

	async function handleDeleteTeam() {
		if (!$auth.token || !teamName || !canDeleteTeam) return;
		deleting = true;
		error = '';
		try {
			await deleteTeam(teamName, $auth.token);
			showDelete = false;
			toast.show('Team deleted', 'success');
			goto('/teams');
		} catch (err) {
			error = formatApiError(err);
		} finally {
			deleting = false;
		}
	}

	function openDeleteTeam() {
		deleteConfirmation = '';
		showDelete = true;
	}

	function openEditTeam() {
		if (!team) return;
		editName = team.name;
		editDescription = team.description || '';
		showEdit = true;
	}

	async function handleSaveTeam(e: Event) {
		e.preventDefault();
		if (!$auth.token || !team || savingTeam) return;
		const newName = editName.trim();
		if (!newName) return;
		savingTeam = true;
		try {
			const payload: { name?: string; description?: string } = { description: editDescription };
			if (newName !== team.name) payload.name = newName;
			await updateTeam(teamName, payload, $auth.token);
			showEdit = false;
			toast.show(newName !== team.name ? `Team renamed to ${newName}` : 'Team updated', 'success');
			if (newName !== team.name) {
				goto(`/teams/${encodeURIComponent(newName)}`);
			} else {
				await loadTeam();
			}
		} catch (err) {
			error = formatApiError(err);
		} finally {
			savingTeam = false;
		}
	}

	// Removing a team label strips it from every team paste, and a single
	// Backspace in the label input does it: ask first when a label is in use.
	let pendingLabels: string[] | null = null;
	let pendingRemoved: { name: string; usage: number }[] = [];

	function handleLabelsChange(e: CustomEvent<string[]>) {
		const next = e.detail;
		const removed = (team?.label_details || []).filter((l) => l.usage > 0 && !next.includes(l.name));
		if (removed.length > 0) {
			pendingLabels = next;
			pendingRemoved = removed.map((l) => ({ name: l.name, usage: l.usage }));
			return;
		}
		void saveLabels(next);
	}

	function cancelLabelRemoval() {
		pendingLabels = null;
		// A new array makes LabelInput drop its local edit.
		teamLabels = [...teamLabels];
	}

	function confirmLabelRemoval() {
		const next = pendingLabels;
		pendingLabels = null;
		if (next) void saveLabels(next);
	}

	async function saveLabels(next: string[]) {
		if (!$auth.token || !teamName) return;
		const seq = ++labelsReqSeq;
		savingLabels = true;
		try {
			const updated = await setTeamLabels(teamName, next, $auth.token);
			if (seq === labelsReqSeq) {
				team = updated;
				teamLabels = updated.labels || [];
			}
		} catch (err) {
			if (seq === labelsReqSeq) {
				error = formatApiError(err);
				await loadTeam();
			}
		} finally {
			if (seq === labelsReqSeq) savingLabels = false;
		}
	}

	$: teamLabelColors = Object.fromEntries((team?.label_details || []).map((l) => [l.name, l.color || '']));

	async function handleLabelColor(labelId: number, color: string) {
		if (!$auth.token) return;
		try {
			const info = await updateLabel(labelId, { color }, $auth.token);
			if (team?.label_details) {
				team.label_details = team.label_details.map((l) => (l.id === labelId ? { ...l, color } : l));
				team = team;
			}
			void info;
		} catch (err) {
			toast.error(formatApiError(err));
		}
	}
</script>

<svelte:head>
	<title>{team?.name || 'Team'} - GopherBin</title>
</svelte:head>

{#if loading}
	<Spinner />
{:else if error && !team}
	<div class="p-4 bg-red-100 dark:bg-red-900 text-red-700 dark:text-red-200 rounded-md">
		{error}
	</div>
{:else if team}
	<div class="space-y-6">
		<div class="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4">
			<div>
				<div class="flex items-center gap-2">
					<h1 class="text-2xl sm:text-3xl font-bold text-gray-900 dark:text-gray-100">{team.name}</h1>
				</div>
				{#if team.description}
					<p class="text-sm text-gray-700 dark:text-gray-300 mt-1 max-w-2xl">{team.description}</p>
				{/if}
				<p class="text-sm text-gray-600 dark:text-gray-400 mt-1">
					{#if isTeamOwner}
						You own this team
					{:else}
						Owned by {team.owner.full_name || team.owner.username} ({team.owner.username})
					{/if}
				</p>
			</div>
			<div class="flex gap-2">
				{#if isTeamOwner}
					<Button on:click={openEditTeam} variant="secondary">Edit</Button>
					{#if !transferPending}
						<Button on:click={() => (showTransfer = true)} variant="secondary">
							<ArrowLeftRight class="w-4 h-4 mr-1 inline" />
							Transfer
						</Button>
					{/if}
					<Button on:click={openDeleteTeam} variant="danger">Delete team</Button>
				{:else if isMember}
					<Button on:click={() => (showLeave = true)} variant="danger">
						<LogOut class="w-4 h-4 mr-1 inline" />
						Leave team
					</Button>
				{/if}
			</div>
		</div>

		{#if error}
			<div class="p-3 bg-red-100 dark:bg-red-900 text-red-700 dark:text-red-200 rounded-md">
				{error}
			</div>
		{/if}

		{#if team.my_pending_transfer}
			<div class="p-4 bg-indigo-50 dark:bg-indigo-900/30 border border-indigo-300 dark:border-indigo-700 rounded-lg flex flex-col sm:flex-row sm:items-center gap-4">
				<div class="flex-1">
					<p class="font-medium text-gray-900 dark:text-gray-100">
						{team.owner.full_name || team.owner.username} wants to transfer ownership of this team to you.
					</p>
					<p class="text-sm text-gray-600 dark:text-gray-400">
						Accepting makes you the owner: you can rename and delete the team, manage all roles,
						and you become the only person who can remove admins. They will stay on the team as admin.
					</p>
				</div>
				<div class="flex gap-2 shrink-0">
					<Button on:click={() => handleTransferAction('decline')} variant="secondary" disabled={transferBusy}>
						Decline
					</Button>
					<Button on:click={() => handleTransferAction('accept')} variant="success" disabled={transferBusy}>
						Accept ownership
					</Button>
				</div>
			</div>
		{:else if transferPending && isTeamOwner}
			<div class="p-4 bg-amber-50 dark:bg-amber-900/30 border border-amber-300 dark:border-amber-700 rounded-lg flex flex-col sm:flex-row sm:items-center gap-4">
				<div class="flex-1">
					<p class="font-medium text-gray-900 dark:text-gray-100">
						Ownership transfer pending: waiting for {team.transfer_to?.full_name || team.transfer_to?.username} to respond.
					</p>
					<p class="text-sm text-gray-600 dark:text-gray-400">
						You remain the owner until they accept. You can cancel the offer at any time.
					</p>
				</div>
				<div class="flex gap-2 shrink-0">
					<Button on:click={() => handleTransferAction('cancel')} variant="secondary" disabled={transferBusy}>
						Cancel offer
					</Button>
				</div>
			</div>
		{/if}

		{#if isInvited}
			<div class="p-4 bg-amber-50 dark:bg-amber-900/30 border border-amber-300 dark:border-amber-700 rounded-lg flex flex-col sm:flex-row sm:items-center gap-4">
				<div class="flex-1">
					<p class="font-medium text-gray-900 dark:text-gray-100">You have been invited to this team.</p>
					<p class="text-sm text-gray-600 dark:text-gray-400">
						{#if myInviter}Invited by {myInviter}.{/if} You cannot see the team's pastes until
						you accept.
					</p>
				</div>
				<div class="flex gap-2 shrink-0">
					<Button on:click={() => (showDecline = true)} variant="secondary" disabled={actionInProgress}>
						Decline
					</Button>
					<Button on:click={handleAccept} variant="success" disabled={actionInProgress}>Accept</Button>
				</div>
			</div>
		{/if}

		{#if canManageMembers}
			<form on:submit={handleAddMember} class="flex flex-col sm:flex-row sm:items-end gap-2 max-w-xl">
				<div class="flex-1 min-w-0">
					<UserSearchInput
						token={$auth.token || ''}
						team={teamName || ''}
						placeholder="Search user to invite, or type an exact username/email"
						disabled={adding}
						on:select={(e) => (newMember = e.detail.username)}
						bind:value={newMember}
					/>
				</div>
				<div>
					<label class="block text-xs text-gray-500 dark:text-gray-400 mb-1" for="invite-role">Role</label>
					<select
						id="invite-role"
						bind:value={inviteRole}
						class="h-10 px-2 rounded-md border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-800 text-sm text-gray-900 dark:text-gray-100"
					>
						<option value="member">Member</option>
						<option value="viewer">Viewer (read-only)</option>
						{#if isTeamOwner}
							<option value="admin">Admin (can invite & remove)</option>
						{/if}
					</select>
				</div>
				<Button type="submit" variant="success" disabled={!newMember.trim() || adding}>
					Send invite
				</Button>
			</form>
		{/if}

		{#if team.stats && !isInvited}
			<div class="grid grid-cols-2 sm:grid-cols-4 gap-3">
				<div class="p-4 bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-700 rounded-lg">
					<div class="text-2xl font-bold text-gray-900 dark:text-gray-100">{team.stats.members}</div>
					<div class="text-xs text-gray-500 dark:text-gray-400 uppercase mt-1">Members</div>
				</div>
				{#if isTeamOwner}
					<div class="p-4 bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-700 rounded-lg">
						<div class="text-2xl font-bold text-gray-900 dark:text-gray-100">{team.stats.pending}</div>
						<div class="text-xs text-gray-500 dark:text-gray-400 uppercase mt-1">Pending invites</div>
					</div>
				{/if}
				<div class="p-4 bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-700 rounded-lg">
					<div class="text-2xl font-bold text-gray-900 dark:text-gray-100">{team.stats.pastes}</div>
					<div class="text-xs text-gray-500 dark:text-gray-400 uppercase mt-1">Pastes</div>
				</div>
				<div class="p-4 bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-700 rounded-lg">
					<div class="text-2xl font-bold text-gray-900 dark:text-gray-100">{team.stats.contributors}</div>
					<div class="text-xs text-gray-500 dark:text-gray-400 uppercase mt-1">Contributors</div>
				</div>
			</div>
		{/if}

		{#if canEditLabels && team}
			<div class="p-4 bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-700 rounded-lg space-y-2">
				<h2 class="text-sm font-semibold text-gray-900 dark:text-gray-100">
				Team labels
				{#if savingLabels}
					<span class="ml-1 text-xs font-normal text-gray-400">Saving…</span>
				{/if}
			</h2>
				<p class="text-xs text-gray-500 dark:text-gray-400">
					Shared vocabulary for labelling this team's pastes. Removing a label also removes it from
					the team's pastes.
				</p>
				<LabelInput
					labels={teamLabels}
					suggestions={teamLabels}
					colorMap={teamLabelColors}
					on:change={handleLabelsChange}
				/>
				{#if team.label_details && team.label_details.length > 0}
					<div class="flex flex-wrap items-center gap-x-3 gap-y-2 pt-1">
						{#each team.label_details as ld (ld.id)}
							<span class="inline-flex items-center gap-1">
								<label
									class="relative inline-flex cursor-pointer items-center gap-1 rounded-full px-2 py-1 text-xs font-medium ring-1 ring-inset ring-black/5 dark:ring-white/10"
									title="Edit color"
									style={labelStyle(ld.color)}
								>
									<span class={ld.color ? '' : labelColor(ld.name, ld.color)}>
										<PaintBucket class="inline-block h-3 w-3 -translate-y-px" />
									</span>
									<span class={ld.color ? '' : 'text-gray-700 dark:text-gray-200'}>
										{ld.name}
									</span>
									<input
										type="color"
										class="absolute inset-0 h-full w-full cursor-pointer opacity-0"
										aria-label={`Edit color for label ${ld.name}`}
										value={ld.color || '#6b7280'}
										on:change={(e) => handleLabelColor(ld.id, (e.target as HTMLInputElement).value)}
									/>
								</label>
								{#if ld.color}
									<button
										type="button"
										class="text-gray-400 hover:text-gray-600 dark:hover:text-gray-200"
										aria-label={`Reset color of ${ld.name} to automatic`}
										title="Automatic color"
										on:click={() => handleLabelColor(ld.id, '')}
									>
										<RotateCcw class="h-3.5 w-3.5" />
									</button>
								{/if}
							</span>
						{/each}
					</div>
				{/if}
			</div>
		{/if}

		<!-- Desktop table -->
		<div class="hidden md:block bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-700 rounded-lg overflow-hidden">
			<table class="min-w-full divide-y divide-gray-200 dark:divide-gray-700">
				<thead class="bg-gray-50 dark:bg-gray-900">
					<tr>
						<th class="px-4 py-2 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase">
							Full Name
						</th>
						<th class="px-4 py-2 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase">
							Username
						</th>
						<th class="px-4 py-2 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase">
							Email
						</th>
						<th class="px-4 py-2 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase">
							Status
						</th>
						<th class="px-4 py-2 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase">
							Added by
						</th>
						{#if canManageMembers}
							<th class="px-4 py-2 text-right text-xs font-medium text-gray-500 dark:text-gray-400 uppercase">
								{#if isTeamOwner}Role / {/if}Action
							</th>
						{/if}
					</tr>
				</thead>
				<tbody class="divide-y divide-gray-200 dark:divide-gray-700">
					{#each members as member}
						<tr class="hover:bg-gray-50 dark:hover:bg-gray-700">
							<td class="px-4 py-2 text-sm text-gray-900 dark:text-gray-100">
								<span class="flex items-center gap-2">
									{#if member.id === team.owner.id}
										<Crown class="w-4 h-4 text-yellow-500" />
									{/if}
									{member.full_name}
									{#if member.id === currentUserId && !isTeamOwner}
										<span class="text-xs text-gray-500 dark:text-gray-400">(you)</span>
									{/if}
								</span>
							</td>
							<td class="px-4 py-2 text-sm text-gray-900 dark:text-gray-100">{member.username}</td>
							<td class="px-4 py-2 text-sm text-gray-900 dark:text-gray-100">{member.email}</td>
							<td class="px-4 py-2 text-sm">
								{#if member.id === team.owner.id}
									<span class="px-2 py-1 text-xs bg-purple-100 dark:bg-purple-900 text-purple-800 dark:text-purple-200 rounded">
										Owner
									</span>
								{:else if member.status === 'pending'}
									<span class="px-2 py-1 text-xs bg-amber-100 dark:bg-amber-900 text-amber-800 dark:text-amber-200 rounded">
										Invited · {roleLabel(member)}
									</span>
								{:else}
									<span class="px-2 py-1 text-xs rounded {roleBadge(member)}">
										{roleLabel(member)}
									</span>
									{#if team.transfer_to?.id === member.id}
										<span class="ml-1 px-2 py-1 text-xs bg-indigo-100 dark:bg-indigo-900 text-indigo-800 dark:text-indigo-200 rounded">
											Owner-to-be
										</span>
									{/if}
								{/if}
							</td>
							<td class="px-4 py-2 text-sm text-gray-600 dark:text-gray-400">
								{#if member.id !== team.owner.id}
									{member.added_by || '—'}
								{:else}
									—
								{/if}
							</td>
							{#if canManageMembers}
								<td class="px-4 py-2 text-sm text-right">
									<span class="inline-flex items-center justify-end gap-3">
										{#if isTeamOwner && member.id !== team.owner.id}
											<select
												aria-label={`Role for ${member.username}`}
												value={member.role || 'member'}
												class="h-8 px-1 rounded border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-800 text-xs text-gray-900 dark:text-gray-100"
												on:change={(e) => handleSetRole(member, (e.target as HTMLSelectElement).value)}
											>
												<option value="admin">Admin</option>
												<option value="member">Member</option>
												<option value="viewer">Viewer</option>
											</select>
										{/if}
										{#if canRemoveMember(member)}
											<button
												type="button"
												on:click={() => (removingMember = member)}
												class="text-red-600 dark:text-red-400 hover:text-red-700"
												title="Remove from team"
											>
												<Trash2 class="w-4 h-4" />
											</button>
										{/if}
									</span>
								</td>
							{/if}
						</tr>
					{/each}
				</tbody>
			</table>
		</div>

		<!-- Mobile cards -->
		<div class="md:hidden space-y-3">
			{#each members as member}
				<div class="bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-700 rounded-lg p-4">
					<div class="flex items-start justify-between gap-3">
						<div class="flex-1 min-w-0">
							<div class="flex items-center gap-2 mb-1 flex-wrap">
								{#if member.id === team.owner.id}
									<Crown class="w-4 h-4 text-yellow-500 shrink-0" />
								{/if}
								<span class="font-semibold text-gray-900 dark:text-gray-100 truncate">
									{member.full_name}
								</span>
								{#if member.id === team.owner.id}
									<span class="px-2 py-0.5 text-xs bg-purple-100 dark:bg-purple-900 text-purple-800 dark:text-purple-200 rounded">Owner</span>
								{:else if member.status === 'pending'}
									<span class="px-2 py-0.5 text-xs bg-amber-100 dark:bg-amber-900 text-amber-800 dark:text-amber-200 rounded">Invited · {roleLabel(member)}</span>
								{:else}
									<span class="px-2 py-0.5 text-xs rounded {roleBadge(member)}">{roleLabel(member)}</span>
									{#if team.transfer_to?.id === member.id}
										<span class="px-2 py-0.5 text-xs bg-indigo-100 dark:bg-indigo-900 text-indigo-800 dark:text-indigo-200 rounded">Owner-to-be</span>
									{/if}
								{/if}
							</div>
							<div class="text-sm text-gray-600 dark:text-gray-400 space-y-0.5">
								<div>{member.username} · {member.email}</div>
								{#if member.id !== team.owner.id && member.added_by}
									<div class="text-xs">Invited by {member.added_by}</div>
								{/if}
							</div>
						</div>
						{#if canManageMembers && member.id !== team.owner.id}
							<div class="flex flex-col items-end gap-2 shrink-0">
								{#if isTeamOwner}
									<select
										aria-label={`Role for ${member.username}`}
										value={member.role || 'member'}
										class="h-8 px-1 rounded border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-800 text-xs text-gray-900 dark:text-gray-100"
										on:change={(e) => handleSetRole(member, (e.target as HTMLSelectElement).value)}
									>
										<option value="admin">Admin</option>
										<option value="member">Member</option>
										<option value="viewer">Viewer</option>
									</select>
								{/if}
								{#if canRemoveMember(member)}
									<button
										type="button"
										on:click={() => (removingMember = member)}
										class="text-red-600 dark:text-red-400 hover:text-red-700"
										title="Remove from team"
									>
										<Trash2 class="w-4 h-4" />
									</button>
								{/if}
							</div>
						{/if}
					</div>
				</div>
			{/each}
		</div>
	</div>
{/if}

<Modal show={showDelete} onClose={() => (showDelete = false)}>
	<div class="space-y-4">
		<h2 class="text-xl font-bold text-gray-900 dark:text-gray-100">Delete team {team?.name}?</h2>
		<div class="p-3 bg-red-50 dark:bg-red-900/30 border border-red-200 dark:border-red-800 rounded-md space-y-2 text-sm text-gray-700 dark:text-gray-300">
			<p class="font-medium text-gray-900 dark:text-gray-100">Deleting this team will:</p>
			<ul class="list-disc list-inside space-y-1 text-gray-600 dark:text-gray-400">
				<li>permanently delete <strong>every paste belonging to {team?.name}</strong>, including pastes created by its members;</li>
				<li>remove all {members.length} member{members.length === 1 ? '' : 's'} from the team;</li>
				<li>revoke all pending invitations.</li>
			</ul>
			<p>This operation cannot be undone.</p>
		</div>
		<div>
			<label class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1" for="team-delete-confirm">
				Type <strong>{teamName}</strong> to confirm
			</label>
			<Input id="team-delete-confirm" bind:value={deleteConfirmation} placeholder={teamName} />
		</div>
		<div class="flex justify-end gap-2">
			<Button on:click={() => (showDelete = false)} variant="secondary">Cancel</Button>
			<Button on:click={handleDeleteTeam} variant="danger" disabled={!canDeleteTeam || deleting}>
				{deleting ? 'Deleting...' : 'Delete team'}
			</Button>
		</div>
	</div>
</Modal>

<Modal show={!!removingMember} onClose={() => (removingMember = null)}>
	<div class="space-y-4">
		<h2 class="text-xl font-bold text-gray-900 dark:text-gray-100">
			Remove {removingMember?.full_name || removingMember?.username} from team?
		</h2>
		<div class="p-3 bg-amber-50 dark:bg-amber-900/30 border border-amber-200 dark:border-amber-800 rounded-md space-y-2 text-sm text-gray-700 dark:text-gray-300">
			<ul class="list-disc list-inside space-y-1 text-gray-600 dark:text-gray-400">
				<li>They will lose access to all pastes of <strong>{team?.name}</strong>.</li>
				<li>Any pastes they created for the team will remain but will no longer be visible to them.</li>
				{#if removingMember?.status === 'pending'}
					<li>Their pending invitation will be revoked.</li>
				{/if}
			</ul>
		</div>
		<div class="flex justify-end gap-2">
			<Button on:click={() => (removingMember = null)} variant="secondary">Cancel</Button>
			<Button on:click={confirmRemoveMember} variant="danger" disabled={actionInProgress}>
				Remove
			</Button>
		</div>
	</div>
</Modal>

<Modal show={!!pendingLabels} onClose={cancelLabelRemoval}>
	<div class="space-y-4">
		<h2 class="text-xl font-bold text-gray-900 dark:text-gray-100">
			Remove team label{pendingRemoved.length === 1 ? '' : 's'}?
		</h2>
		<div class="p-3 bg-amber-50 dark:bg-amber-900/30 border border-amber-200 dark:border-amber-800 rounded-md text-sm">
			<ul class="list-disc list-inside space-y-1 text-gray-600 dark:text-gray-400">
				{#each pendingRemoved as removed (removed.name)}
					<li>
						<strong>{removed.name}</strong> will be removed from {removed.usage} team
						paste{removed.usage === 1 ? '' : 's'}.
					</li>
				{/each}
			</ul>
		</div>
		<div class="flex justify-end gap-2">
			<Button on:click={cancelLabelRemoval} variant="secondary">Cancel</Button>
			<Button on:click={confirmLabelRemoval} variant="danger">Remove</Button>
		</div>
	</div>
</Modal>

<Modal show={showLeave} onClose={() => (showLeave = false)}>
	<div class="space-y-4">
		<h2 class="text-xl font-bold text-gray-900 dark:text-gray-100">Leave {teamName}?</h2>
		<div class="p-3 bg-amber-50 dark:bg-amber-900/30 border border-amber-200 dark:border-amber-800 rounded-md space-y-2 text-sm text-gray-700 dark:text-gray-300">
			<ul class="list-disc list-inside space-y-1 text-gray-600 dark:text-gray-400">
				<li>You will lose access to all pastes of <strong>{teamName}</strong>, including the ones you created.</li>
				<li>Pastes you created for the team will remain and stay visible to the team.</li>
				<li>The team owner or an admin will have to invite you again if you want to rejoin.</li>
			</ul>
		</div>
		<div class="flex justify-end gap-2">
			<Button on:click={() => (showLeave = false)} variant="secondary">Cancel</Button>
			<Button on:click={confirmLeave} variant="danger" disabled={leaving}>
				{leaving ? 'Leaving...' : 'Leave team'}
			</Button>
		</div>
	</div>
</Modal>

<Modal show={showDecline} onClose={() => (showDecline = false)}>
	<div class="space-y-4">
		<h2 class="text-xl font-bold text-gray-900 dark:text-gray-100">Decline invitation?</h2>
		<div class="p-3 bg-amber-50 dark:bg-amber-900/30 border border-amber-200 dark:border-amber-800 rounded-md space-y-2 text-sm text-gray-700 dark:text-gray-300">
			<ul class="list-disc list-inside space-y-1 text-gray-600 dark:text-gray-400">
				<li>You will not join <strong>{teamName}</strong> and cannot see its pastes.</li>
				<li>The team owner will have to invite you again if you change your mind.</li>
			</ul>
		</div>
		<div class="flex justify-end gap-2">
			<Button on:click={() => (showDecline = false)} variant="secondary">Keep invitation</Button>
			<Button on:click={confirmDecline} variant="danger" disabled={actionInProgress}>Decline</Button>
		</div>
	</div>
</Modal>

<Modal show={showTransfer} onClose={() => (showTransfer = false)}>
	<div class="space-y-4">
		<h2 class="text-xl font-bold text-gray-900 dark:text-gray-100">Transfer ownership of {teamName}</h2>
		<div class="p-3 bg-amber-50 dark:bg-amber-900/30 border border-amber-200 dark:border-amber-800 rounded-md space-y-2 text-sm text-gray-700 dark:text-gray-300">
			<ul class="list-disc list-inside space-y-1 text-gray-600 dark:text-gray-400">
				<li>The transfer completes only when the other person <strong>accepts</strong>; until then you stay in charge.</li>
				<li>The new owner becomes the only user who can remove admins, and can rename or delete the team.</li>
				<li>You will remain on the team with the <strong>admin</strong> role.</li>
			</ul>
		</div>
		<div>
			<label class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1" for="transfer-target">
				New owner
			</label>
			<select
				id="transfer-target"
				bind:value={transferTarget}
				class="w-full h-10 px-2 rounded-md border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-800 text-sm text-gray-900 dark:text-gray-100"
			>
				<option value="">Select a team member…</option>
				{#each members.filter((m) => m.status === 'active' && m.id !== team?.owner.id) as m (m.id)}
					<option value={m.username}>{m.full_name} ({m.username}) — {m.role || 'member'}</option>
				{/each}
			</select>
		</div>
		<div class="flex justify-end gap-2">
			<Button on:click={() => (showTransfer = false)} variant="secondary">Cancel</Button>
			<Button on:click={confirmTransfer} variant="success" disabled={!transferTarget || transferring}>
				{transferring ? 'Offering...' : 'Offer ownership'}
			</Button>
		</div>
	</div>
</Modal>

<Modal show={showEdit} onClose={() => (showEdit = false)}>
	<form on:submit={handleSaveTeam} class="space-y-4">
		<h2 class="text-xl font-bold text-gray-900 dark:text-gray-100">Edit team</h2>
		<div>
			<label class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1" for="team-edit-name">Name</label>
			<Input id="team-edit-name" bind:value={editName} />
			<p class="text-xs text-gray-500 dark:text-gray-400 mt-1">
				Renaming changes the team URL; links to the old name will stop working.
			</p>
		</div>
		<div>
			<label class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1" for="team-edit-desc">Description</label>
			<textarea
				id="team-edit-desc"
				bind:value={editDescription}
				rows="3"
				maxlength="254"
				class="w-full px-3 py-2 border rounded-md bg-white dark:bg-gray-800 text-gray-900 dark:text-gray-100 border-gray-300 dark:border-gray-600 focus:outline-none focus:ring-2 focus:ring-blue-500"
				placeholder="What is this team for?"
			></textarea>
		</div>
		<div class="flex justify-end gap-2">
			<Button on:click={() => (showEdit = false)} variant="secondary">Cancel</Button>
			<Button type="submit" disabled={!editName.trim() || savingTeam}>
				{savingTeam ? 'Saving...' : 'Save'}
			</Button>
		</div>
	</form>
</Modal>
