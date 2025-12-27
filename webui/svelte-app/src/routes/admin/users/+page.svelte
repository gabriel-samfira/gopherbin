<script lang="ts">
	import { goto } from '$app/navigation';
	import { onMount } from 'svelte';
	import { auth } from '$lib/stores/auth';
	import { listUsers, deleteUser } from '$lib/api/users';
	import Button from '$lib/components/ui/Button.svelte';
	import Spinner from '$lib/components/ui/Spinner.svelte';
	import Modal from '$lib/components/ui/Modal.svelte';
	import type { User } from '$lib/types/user';
	import type { ApiError } from '$lib/types/api';

	let users: User[] = [];
	let loading = true;
	let error = '';
	let page = 1;
	let totalPages = 1;
	let maxResults = 20;
	let deletingUser: User | null = null;

	async function loadUsers() {
		if (!$auth.token || !$auth.isAdmin) {
			goto('/');
			return;
		}

		loading = true;
		error = '';

		try {
			const response = await listUsers(page, maxResults, $auth.token);
			users = response.users || [];
			totalPages = response.total_pages;
		} catch (err) {
			const apiError = err as ApiError;
			error = apiError.details || apiError.error || 'Failed to load users';
		} finally {
			loading = false;
		}
	}

	onMount(loadUsers);

	function handlePageChange(newPage: number) {
		if (newPage < 1 || newPage > totalPages) return;
		page = newPage;
		loadUsers();
	}

	function initDelete(user: User) {
		deletingUser = user;
	}

	async function confirmDelete() {
		if (!deletingUser || !$auth.token) return;

		try {
			await deleteUser(deletingUser.id, $auth.token);
			deletingUser = null;
			loadUsers();
		} catch (err) {
			const apiError = err as ApiError;
			error = apiError.details || apiError.error || 'Failed to delete user';
		}
	}
</script>

<svelte:head>
	<title>Users - GopherBin Admin</title>
</svelte:head>

{#if !$auth.isAdmin}
	<div class="p-4 bg-red-100 dark:bg-red-900 text-red-700 dark:text-red-200 rounded-md">
		Access denied. Admin privileges required.
	</div>
{:else}
	<div class="space-y-6">
		<div class="flex justify-between items-center">
			<h1 class="text-3xl font-bold text-gray-900 dark:text-gray-100">User Management</h1>
			<Button on:click={() => goto('/admin/users/new')} variant="primary">
				Create New User
			</Button>
		</div>

		{#if error}
			<div class="p-3 bg-red-100 dark:bg-red-900 text-red-700 dark:text-red-200 rounded-md">
				{error}
			</div>
		{/if}

		{#if loading}
			<Spinner />
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

			<div class="bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-700 rounded-lg overflow-hidden">
				<table class="min-w-full divide-y divide-gray-200 dark:divide-gray-700">
					<thead class="bg-gray-50 dark:bg-gray-900">
						<tr>
							<th class="px-6 py-3 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase">
								Username
							</th>
							<th class="px-6 py-3 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase">
								Full Name
							</th>
							<th class="px-6 py-3 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase">
								Email
							</th>
							<th class="px-6 py-3 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase">
								Status
							</th>
							<th class="px-6 py-3 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase">
								Actions
							</th>
						</tr>
					</thead>
					<tbody class="divide-y divide-gray-200 dark:divide-gray-700">
						{#each users as user}
							<tr
								class="hover:bg-gray-50 dark:hover:bg-gray-700 cursor-pointer"
								on:click={() => goto(`/admin/users/${user.id}`, { state: { user } })}
								on:keydown={(e) => e.key === 'Enter' && goto(`/admin/users/${user.id}`, { state: { user } })}
								role="button"
								tabindex="0"
							>
								<td class="px-6 py-4 whitespace-nowrap text-sm text-gray-900 dark:text-gray-100">
									{user.username}
									{#if user.is_admin}
										<span class="ml-2 px-2 py-1 text-xs bg-blue-100 dark:bg-blue-900 text-blue-800 dark:text-blue-200 rounded">
											Admin
										</span>
									{/if}
								</td>
								<td class="px-6 py-4 whitespace-nowrap text-sm text-gray-900 dark:text-gray-100">
									{user.full_name}
								</td>
								<td class="px-6 py-4 whitespace-nowrap text-sm text-gray-900 dark:text-gray-100">
									{user.email}
								</td>
								<td class="px-6 py-4 whitespace-nowrap text-sm">
									<span
										class="px-2 py-1 rounded {user.enabled
											? 'bg-green-100 dark:bg-green-900 text-green-800 dark:text-green-200'
											: 'bg-red-100 dark:bg-red-900 text-red-800 dark:text-red-200'}"
									>
										{user.enabled ? 'Enabled' : 'Disabled'}
									</span>
								</td>
								<td class="px-6 py-4 whitespace-nowrap text-sm">
									<div
										on:click={(e) => e.stopPropagation()}
										on:keydown={(e) => e.stopPropagation()}
										role="none"
									>
										<Button on:click={() => initDelete(user)} variant="danger">Delete</Button>
									</div>
								</td>
							</tr>
						{/each}
					</tbody>
				</table>
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

	<Modal show={!!deletingUser} onClose={() => (deletingUser = null)}>
		<div class="space-y-4">
			<h2 class="text-xl font-bold text-gray-900 dark:text-gray-100">Confirm Delete</h2>
			<p class="text-gray-700 dark:text-gray-300">
				Are you sure you want to delete user <strong>{deletingUser?.username}</strong>?
			</p>
			<div class="flex justify-end gap-2">
				<Button on:click={() => (deletingUser = null)} variant="secondary">Cancel</Button>
				<Button on:click={confirmDelete} variant="danger">Delete</Button>
			</div>
		</div>
	</Modal>
{/if}
