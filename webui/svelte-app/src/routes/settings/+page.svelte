<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { auth } from '$lib/stores/auth';
	import { getMe, updateMe, updateUser } from '$lib/api/users';
	import { getMyLabels, updateLabel, deleteLabel } from '$lib/api/pastes';
	import Button from '$lib/components/ui/Button.svelte';
	import Spinner from '$lib/components/ui/Spinner.svelte';
	import Toggle from '$lib/components/ui/Toggle.svelte';
	import { toast } from '$lib/stores/toast';
	import { formatApiError } from '$lib/utils/errors';
	import { labelColor, labelStyle } from '$lib/utils/labelColor';
	import type { User } from '$lib/types/user';
	import type { LabelInfo } from '$lib/types/paste';

	let me: User | null = null;
	let loading = true;
	let error = '';
	let discoverable = true;
	let saving = false;

	let myLabels: LabelInfo[] = [];
	let labelsError = '';
	let editingId: number | null = null;
	let editName = '';
	let busyId: number | null = null;
	let confirmDeleteId: number | null = null;

	let currentPassword = '';
	let newPassword = '';
	let confirmPassword = '';
	let changingPassword = false;
	let passwordError = '';

	$: dirty = me !== null && discoverable !== me.discoverable;
	$: passwordFormValid =
		currentPassword !== '' && newPassword !== '' && newPassword === confirmPassword;
	$: passwordMismatch = confirmPassword !== '' && newPassword !== confirmPassword;

	onMount(async () => {
		if (!$auth.token) {
			goto('/login?next=%2Fsettings');
			return;
		}
		try {
			me = await getMe($auth.token);
			discoverable = me.discoverable;
		} catch (err) {
			error = formatApiError(err);
		} finally {
			loading = false;
		}
		loadLabels();
	});

	async function loadLabels() {
		if (!$auth.token) return;
		try {
			myLabels = await getMyLabels($auth.token);
		} catch (err) {
			labelsError = formatApiError(err);
		}
	}

	async function saveLabelColor(label: LabelInfo, color: string) {
		if (!$auth.token || busyId !== null) return;
		busyId = label.id;
		labelsError = '';
		try {
			const info = await updateLabel(label.id, { color }, $auth.token);
			myLabels = myLabels.map((l) => (l.id === label.id ? info : l));
		} catch (err) {
			labelsError = formatApiError(err);
		} finally {
			busyId = null;
		}
	}

	function startRename(label: LabelInfo) {
		editingId = label.id;
		editName = label.name;
	}

	async function commitRename(label: LabelInfo) {
		if (!$auth.token || busyId !== null) return;
		const name = editName.trim();
		editingId = null;
		if (name === '' || name === label.name) return;
		busyId = label.id;
		labelsError = '';
		try {
			await updateLabel(label.id, { name }, $auth.token);
			await loadLabels();
			toast.show('Label renamed', 'success');
		} catch (err) {
			labelsError = formatApiError(err);
		} finally {
			busyId = null;
		}
	}

	async function removeLabel(label: LabelInfo) {
		if (!$auth.token || busyId !== null) return;
		confirmDeleteId = null;
		busyId = label.id;
		labelsError = '';
		try {
			await deleteLabel(label.id, $auth.token);
			myLabels = myLabels.filter((l) => l.id !== label.id);
			toast.show(`Label "${label.name}" removed`, 'success');
		} catch (err) {
			labelsError = formatApiError(err);
		} finally {
			busyId = null;
		}
	}

	async function save() {
		if (!$auth.token || saving || !dirty) return;
		saving = true;
		error = '';
		try {
			me = await updateMe(discoverable, $auth.token);
			discoverable = me.discoverable;
			toast.show('Settings saved', 'success');
		} catch (err) {
			error = formatApiError(err);
		} finally {
			saving = false;
		}
	}

	async function changePassword() {
		if (!$auth.token || !me || changingPassword || !passwordFormValid) return;
		changingPassword = true;
		passwordError = '';
		try {
			// The server verifies current_password itself; a wrong one comes
			// back as 401 with "current password is incorrect" details.
			await updateUser(me.id, { password: newPassword, current_password: currentPassword }, $auth.token);
			currentPassword = '';
			newPassword = '';
			confirmPassword = '';
			// The server bumps the account's update stamp, which invalidates the
			// current token, so send the user back to the login screen.
			toast.show('Password changed, please log in again', 'success');
			auth.logout();
			goto('/login');
		} catch (err) {
			passwordError = formatApiError(err);
		} finally {
			changingPassword = false;
		}
	}
</script>

<svelte:head>
	<title>Settings - GopherBin</title>
</svelte:head>

{#if loading}
	<Spinner />
{:else}
	<div class="max-w-2xl mx-auto space-y-6">
		<h1 class="text-2xl sm:text-3xl font-bold text-gray-900 dark:text-gray-100">Account settings</h1>

		{#if error}
			<div class="p-3 bg-red-100 dark:bg-red-900 text-red-700 dark:text-red-200 rounded-md">{error}</div>
		{/if}

		{#if me}
			<div class="p-4 bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-700 rounded-lg space-y-1">
				<h2 class="font-semibold text-gray-900 dark:text-gray-100">{me.full_name}</h2>
				<p class="text-sm text-gray-600 dark:text-gray-400">@{me.username} · {me.email}</p>
			</div>

			<div class="p-4 bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-700 rounded-lg space-y-3">
				<h2 class="font-semibold text-gray-900 dark:text-gray-100">Team invitations</h2>
				<div class="flex items-start justify-between gap-4">
					<div>
						<p class="text-sm text-gray-700 dark:text-gray-300">Appear in team-invite search</p>
						<p class="text-xs text-gray-500 dark:text-gray-400 mt-1 max-w-md">
							When enabled, teammates can find your account while typing in the team-invite box.
							When disabled, you never appear in the suggestions and can only be invited by someone
							who already knows your exact username or email address.
						</p>
					</div>
					<Toggle bind:checked={discoverable} label="" />
				</div>
				<div class="flex justify-end">
					<Button on:click={save} disabled={!dirty || saving}>
						{saving ? 'Saving...' : 'Save'}
					</Button>
				</div>
			</div>

			<div class="p-4 bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-700 rounded-lg space-y-3">
				<h2 class="font-semibold text-gray-900 dark:text-gray-100">Change password</h2>
				<p class="text-xs text-gray-500 dark:text-gray-400">
					Your current password is verified before the new one is saved. Passwords are checked for
					strength; weak ones are rejected. You will be signed out after a successful change.
				</p>
				{#if passwordError}
					<div class="p-2 bg-red-100 dark:bg-red-900 text-red-700 dark:text-red-200 rounded-md text-sm">
						{passwordError}
					</div>
				{/if}
				<div class="grid gap-3 sm:grid-cols-3">
					<label class="block">
						<span class="text-xs text-gray-600 dark:text-gray-400">Current password</span>
						<input
							type="password"
							autocomplete="current-password"
							class="mt-1 w-full h-10 px-2 text-sm border border-gray-300 dark:border-gray-600 rounded-md bg-white dark:bg-gray-900 text-gray-900 dark:text-gray-100"
							bind:value={currentPassword}
						/>
					</label>
					<label class="block">
						<span class="text-xs text-gray-600 dark:text-gray-400">New password</span>
						<input
							type="password"
							autocomplete="new-password"
							class="mt-1 w-full h-10 px-2 text-sm border border-gray-300 dark:border-gray-600 rounded-md bg-white dark:bg-gray-900 text-gray-900 dark:text-gray-100"
							bind:value={newPassword}
						/>
					</label>
					<label class="block">
						<span class="text-xs text-gray-600 dark:text-gray-400">Confirm new password</span>
						<input
							type="password"
							autocomplete="new-password"
							class="mt-1 w-full h-10 px-2 text-sm border border-gray-300 dark:border-gray-600 rounded-md bg-white dark:bg-gray-900 text-gray-900 dark:text-gray-100"
							bind:value={confirmPassword}
						/>
					</label>
				</div>
				{#if passwordMismatch}
					<p class="text-xs text-red-600 dark:text-red-300">The new passwords do not match.</p>
				{/if}
				<div class="flex justify-end">
					<Button on:click={changePassword} disabled={!passwordFormValid || changingPassword}>
						{changingPassword ? 'Changing...' : 'Change password'}
					</Button>
				</div>
			</div>

			<div class="p-4 bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-700 rounded-lg space-y-3">
				<h2 class="font-semibold text-gray-900 dark:text-gray-100">Your labels</h2>
				<p class="text-xs text-gray-500 dark:text-gray-400">
					Personal labels you have created. Renaming onto an existing label merges them; deleting
					removes the label from every paste.
				</p>
				{#if labelsError}
					<div class="p-2 bg-red-100 dark:bg-red-900 text-red-700 dark:text-red-200 rounded-md text-sm">
						{labelsError}
					</div>
				{/if}
				{#if myLabels.length === 0}
					<p class="text-sm text-gray-500 dark:text-gray-400">No personal labels yet.</p>
				{:else}
					<ul class="divide-y divide-gray-100 dark:divide-gray-700">
						{#each myLabels as label (label.id)}
							<li class="py-2 flex flex-wrap items-center gap-2">
								<input
									type="color"
									class="h-10 w-10 shrink-0 p-1 border border-gray-300 dark:border-gray-600 rounded bg-transparent cursor-pointer"
									aria-label="Color for label {label.name}"
									value={label.color || '#6b7280'}
									on:change={(e) => saveLabelColor(label, (e.target as HTMLInputElement).value)}
								/>
								{#if label.color}
									<button
										type="button"
										class="text-xs text-gray-400 hover:text-gray-600 dark:hover:text-gray-200 underline"
										on:click={() => saveLabelColor(label, '')}
									>auto</button>
								{/if}
								{#if editingId === label.id}
									<input
										type="text"
										class="flex-1 min-w-[8rem] h-10 px-2 text-sm border border-gray-300 dark:border-gray-600 rounded-md bg-white dark:bg-gray-900 text-gray-900 dark:text-gray-100"
										bind:value={editName}
										on:keydown={(e) => e.key === 'Enter' && commitRename(label)}
										on:blur={() => commitRename(label)}
									/>
								{:else}
									<span class="px-2 py-0.5 rounded-full text-xs font-medium {labelColor(label.name, label.color)}" style={labelStyle(label.color)}>
										{label.name}
									</span>
									<span class="text-xs text-gray-500 dark:text-gray-400">
										{label.usage} paste{label.usage === 1 ? '' : 's'}
									</span>
									{#if busyId === label.id}
										<span class="text-xs text-gray-400">Saving…</span>
									{/if}
									<span class="flex-1"></span>
									<div class="flex items-center gap-3 ml-auto">
										{#if confirmDeleteId === label.id}
											<span class="text-xs text-red-600 dark:text-red-300">
												Remove from {label.usage} paste{label.usage === 1 ? '' : 's'}?
											</span>
											<button
												type="button"
												class="text-xs font-medium text-red-600 dark:text-red-300 hover:underline"
												on:click={() => removeLabel(label)}
											>Delete</button>
											<button
												type="button"
												class="text-xs text-gray-500 hover:underline"
												on:click={() => (confirmDeleteId = null)}
											>Cancel</button>
										{:else}
											<button
												type="button"
												class="text-xs text-blue-600 dark:text-blue-300 hover:underline"
												on:click={() => startRename(label)}
											>Rename</button>
											<button
												type="button"
												class="text-xs text-red-600 dark:text-red-300 hover:underline"
												on:click={() => (confirmDeleteId = label.id)}
											>Delete</button>
										{/if}
									</div>
								{/if}
							</li>
						{/each}
					</ul>
				{/if}
			</div>
		{/if}
	</div>
{/if}
