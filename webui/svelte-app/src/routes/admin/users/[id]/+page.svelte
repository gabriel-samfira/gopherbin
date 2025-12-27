<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/stores';
	import { onMount } from 'svelte';
	import { auth } from '$lib/stores/auth';
	import { toast } from '$lib/stores/toast';
	import { updateUser, deleteUser } from '$lib/api/users';
	import Button from '$lib/components/ui/Button.svelte';
	import Input from '$lib/components/ui/Input.svelte';
	import Modal from '$lib/components/ui/Modal.svelte';
	import Toggle from '$lib/components/ui/Toggle.svelte';
	import type { User, UserUpdate } from '$lib/types/user';
	import type { ApiError } from '$lib/types/api';

	let user: User | null = null;
	let error = '';

	// User info form
	let fullName = '';
	let username = '';
	let email = '';
	let enabled = true;
	let isAdmin = false;

	// Password reset form
	let newPassword = '';
	let confirmPassword = '';
	let passwordError = '';

	// Delete confirmation
	let deleteConfirmation = '';
	let showDeleteModal = false;

	$: userId = $page.params.id;
	$: passwordsMatch = newPassword && newPassword === confirmPassword && newPassword.length >= 8;

	onMount(async () => {
		if (!$auth.token || !$auth.isAdmin) {
			goto('/');
			return;
		}

		// Get user data from SvelteKit's page state
		const state = $page.state as { user?: User } | undefined;
		if (state?.user) {
			user = state.user;
			fullName = user.full_name;
			username = user.username;
			email = user.email;
			enabled = user.enabled;
			isAdmin = user.is_admin;
		} else {
			// If no state, redirect back to user list
			error = 'User data not found. Please select a user from the list.';
			setTimeout(() => goto('/admin/users'), 2000);
		}
	});

	async function handleUpdateUserInfo() {
		if (!$auth.token || !userId) return;

		error = '';
		try {
			const updates: UserUpdate = {
				full_name: fullName,
				email: email,
				enabled: enabled,
				is_admin: isAdmin
			};
			await updateUser(userId, updates, $auth.token);
			toast.show('User info updated successfully', 'success');
			// Update local user object
			if (user) {
				user.enabled = enabled;
				user.is_admin = isAdmin;
			}
		} catch (err) {
			const apiError = err as ApiError;
			error = apiError.details || apiError.error || 'Failed to update user info';
		}
	}

	async function handleResetPassword() {
		if (!$auth.token || !userId || !passwordsMatch) return;

		passwordError = '';
		try {
			const updates: UserUpdate = {
				password: newPassword
			};
			await updateUser(userId, updates, $auth.token);
			newPassword = '';
			confirmPassword = '';
			toast.show('Password reset successfully', 'success');
		} catch (err) {
			const apiError = err as ApiError;
			passwordError = apiError.details || apiError.error || 'Failed to reset password';
		}
	}

	async function handleDeleteUser() {
		if (!$auth.token || !userId) return;

		try {
			await deleteUser(userId, $auth.token);
			goto('/admin/users');
		} catch (err) {
			const apiError = err as ApiError;
			error = apiError.details || apiError.error || 'Failed to delete user';
		}
	}
</script>

<svelte:head>
	<title>Edit User - GopherBin Admin</title>
</svelte:head>

{#if !$auth.isAdmin}
	<div class="p-4 bg-red-100 dark:bg-red-900 text-red-700 dark:text-red-200 rounded-md">
		Access denied. Admin privileges required.
	</div>
{:else if user}
	<div class="space-y-6">
		<div class="flex items-center gap-4">
			<Button on:click={() => goto('/admin/users')} variant="secondary">← Back to Users</Button>
			<h1 class="text-3xl font-bold text-gray-900 dark:text-gray-100">
				Edit User: {user?.username || `#${userId}`}
			</h1>
		</div>

		{#if error}
			<div class="p-3 bg-red-100 dark:bg-red-900 text-red-700 dark:text-red-200 rounded-md">
				{error}
			</div>
		{/if}

		<!-- Update User Info -->
		<div class="bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-700 rounded-lg p-6">
			<h2 class="text-xl font-bold text-gray-900 dark:text-gray-100 mb-4">Update User Info</h2>
			<div class="space-y-4">
				<div>
					<span class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
						Full Name
					</span>
					<Input bind:value={fullName} placeholder="Enter full name" />
				</div>
				<div>
					<span class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
						Username
					</span>
					<Input bind:value={username} placeholder="Enter username" readonly />
				</div>
				<div>
					<span class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
						Email
					</span>
					<Input bind:value={email} type="email" placeholder="Enter email" />
				</div>
				<div class="flex items-center gap-6 pt-2">
					<Toggle bind:checked={enabled} label="Account Enabled" />
					<Toggle bind:checked={isAdmin} label="Admin User" />
				</div>
				<Button on:click={handleUpdateUserInfo} variant="primary">
					Update User Info
				</Button>
			</div>
		</div>

		<!-- Reset Password -->
		<div class="bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-700 rounded-lg p-6">
			<h2 class="text-xl font-bold text-gray-900 dark:text-gray-100 mb-4">Reset Password</h2>
			{#if passwordError}
				<div class="p-3 bg-red-100 dark:bg-red-900 text-red-700 dark:text-red-200 rounded-md mb-4">
					{passwordError}
				</div>
			{/if}
			<div class="space-y-4">
				<div>
					<span class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
						New Password
					</span>
					<Input bind:value={newPassword} type="password" placeholder="Minimum 8 characters" />
				</div>
				<div>
					<span class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
						Confirm Password
					</span>
					<Input bind:value={confirmPassword} type="password" placeholder="Re-enter password" />
				</div>
				{#if newPassword && confirmPassword && !passwordsMatch}
					<p class="text-sm text-red-600 dark:text-red-400">Passwords do not match or are too short (min 8 characters)</p>
				{/if}
				<Button on:click={handleResetPassword} variant="primary" disabled={!passwordsMatch}>
					Reset Password
				</Button>
			</div>
		</div>

		<!-- Danger Zone -->
		<div class="bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-lg p-6">
			<h2 class="text-xl font-bold text-red-900 dark:text-red-200 mb-4">Danger Zone</h2>
			<p class="text-red-700 dark:text-red-300 mb-4">
				WARNING: This operation is permanent and will delete the user and all pastes created by this user.
			</p>
			<p class="text-red-700 dark:text-red-300 mb-4">
				To delete this user, type <strong>DELETE</strong> in the field below and click the delete button.
			</p>
			<div class="space-y-4">
				<Input
					bind:value={deleteConfirmation}
					placeholder="Type DELETE to confirm"
				/>
				<Button
					on:click={() => (showDeleteModal = true)}
					variant="danger"
					disabled={deleteConfirmation !== 'DELETE'}
				>
					Delete User
				</Button>
			</div>
		</div>
	</div>

	<Modal show={showDeleteModal} onClose={() => (showDeleteModal = false)}>
		<div class="space-y-4">
			<h2 class="text-xl font-bold text-gray-900 dark:text-gray-100">Final Confirmation</h2>
			<p class="text-gray-700 dark:text-gray-300">
				Are you absolutely sure you want to delete this user? This action cannot be undone.
			</p>
			<div class="flex justify-end gap-2">
				<Button on:click={() => (showDeleteModal = false)} variant="secondary">Cancel</Button>
				<Button on:click={handleDeleteUser} variant="danger">Yes, Delete User</Button>
			</div>
		</div>
	</Modal>
{/if}
