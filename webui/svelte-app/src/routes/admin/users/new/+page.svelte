<script lang="ts">
	import { goto } from '$app/navigation';
	import { auth } from '$lib/stores/auth';
	import { createUser } from '$lib/api/users';
	import Button from '$lib/components/ui/Button.svelte';
	import Input from '$lib/components/ui/Input.svelte';
	import Spinner from '$lib/components/ui/Spinner.svelte';
	import type { ApiError } from '$lib/types/api';

	let username = '';
	let email = '';
	let password = '';
	let fullName = '';
	let enabled = true;
	let isAdmin = false;
	let loading = false;
	let error = '';

	$: canSubmit =
		username.length > 0 && email.length > 0 && password.length >= 6 && fullName.length > 0;

	async function handleSubmit(e: Event) {
		e.preventDefault();

		if (!canSubmit || !$auth.token) return;

		loading = true;
		error = '';

		try {
			await createUser(
				{
					username,
					email,
					password,
					full_name: fullName,
					enabled,
					is_admin: isAdmin
				},
				$auth.token
			);

			goto('/admin/users');
		} catch (err) {
			const apiError = err as ApiError;
			error = apiError.details || apiError.error || 'Failed to create user';
		} finally {
			loading = false;
		}
	}
</script>

<svelte:head>
	<title>Create User - GopherBin Admin</title>
</svelte:head>

{#if !$auth.isAdmin}
	<div class="p-4 bg-red-100 dark:bg-red-900 text-red-700 dark:text-red-200 rounded-md">
		Access denied. Admin privileges required.
	</div>
{:else}
	<div class="max-w-2xl mx-auto">
		<h1 class="text-3xl font-bold text-gray-900 dark:text-gray-100 mb-6">Create New User</h1>

		{#if error}
			<div class="mb-4 p-3 bg-red-100 dark:bg-red-900 text-red-700 dark:text-red-200 rounded-md">
				{error}
			</div>
		{/if}

		{#if loading}
			<Spinner />
		{:else}
			<form on:submit={handleSubmit} class="space-y-4">
				<div>
					<label for="username" class="block text-sm font-medium mb-1 text-gray-700 dark:text-gray-300">
						Username
					</label>
					<Input id="username" bind:value={username} placeholder="Enter username" />
				</div>

				<div>
					<label for="email" class="block text-sm font-medium mb-1 text-gray-700 dark:text-gray-300">
						Email
					</label>
					<Input id="email" type="email" bind:value={email} placeholder="Enter email" />
				</div>

				<div>
					<label for="fullName" class="block text-sm font-medium mb-1 text-gray-700 dark:text-gray-300">
						Full Name
					</label>
					<Input id="fullName" bind:value={fullName} placeholder="Enter full name" />
				</div>

				<div>
					<label for="password" class="block text-sm font-medium mb-1 text-gray-700 dark:text-gray-300">
						Password
					</label>
					<Input id="password" type="password" bind:value={password} placeholder="Enter password (min 6 characters)" />
				</div>

				<div class="flex items-center gap-6">
					<div class="flex items-center gap-2">
						<input
							id="enabled"
							type="checkbox"
							bind:checked={enabled}
							class="w-4 h-4 text-blue-600 border-gray-300 rounded focus:ring-blue-500"
						/>
						<label for="enabled" class="text-sm font-medium text-gray-700 dark:text-gray-300">
							Enabled
						</label>
					</div>
					<div class="flex items-center gap-2">
						<input
							id="isAdmin"
							type="checkbox"
							bind:checked={isAdmin}
							class="w-4 h-4 text-blue-600 border-gray-300 rounded focus:ring-blue-500"
						/>
						<label for="isAdmin" class="text-sm font-medium text-gray-700 dark:text-gray-300">
							Administrator
						</label>
					</div>
				</div>

				<div class="flex gap-2">
					<Button type="submit" variant="success" disabled={!canSubmit}>Create User</Button>
					<Button on:click={() => goto('/admin/users')} variant="secondary">Cancel</Button>
				</div>
			</form>
		{/if}
	</div>
{/if}
