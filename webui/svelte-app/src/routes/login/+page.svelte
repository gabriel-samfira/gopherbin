<script lang="ts">
	import { goto } from '$app/navigation';
	import { auth } from '$lib/stores/auth';
	import { login } from '$lib/api/auth';
	import Button from '$lib/components/ui/Button.svelte';
	import Input from '$lib/components/ui/Input.svelte';
	import Spinner from '$lib/components/ui/Spinner.svelte';
	import type { ApiError } from '$lib/types/api';

	let username = '';
	let password = '';
	let loading = false;
	let error = '';

	$: canSubmit = username.length > 0 && password.length >= 6;

	async function handleSubmit(e: Event) {
		e.preventDefault();

		if (!canSubmit) return;

		loading = true;
		error = '';

		try {
			const response = await login({ username, password });
			auth.login(response);
			goto('/');
		} catch (err) {
			const apiError = err as ApiError;
			error = apiError.details || apiError.error || 'Login failed';
		} finally {
			loading = false;
		}
	}

	// Redirect if already authenticated
	$: if ($auth.isAuthenticated) {
		goto('/');
	}
</script>

<svelte:head>
	<title>Login - GopherBin</title>
</svelte:head>

<div class="max-w-md mx-auto mt-16">
	<div class="bg-white dark:bg-gray-800 shadow-md rounded-lg p-8">
		<div class="flex flex-col items-center mb-6">
			<img src="/logo.svg" alt="GopherBin" class="h-24 w-auto mb-4" />
			<h1 class="text-2xl font-bold text-center text-gray-900 dark:text-gray-100">
				Welcome to GopherBin
			</h1>
		</div>

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
						Email or Username
					</label>
					<Input id="username" bind:value={username} placeholder="Enter your username or email" />
				</div>

				<div>
					<label for="password" class="block text-sm font-medium mb-1 text-gray-700 dark:text-gray-300">
						Password
					</label>
					<Input
						id="password"
						type="password"
						bind:value={password}
						placeholder="Enter your password"
					/>
				</div>

				<Button type="submit" variant="primary" disabled={!canSubmit} class="w-full">
					Login
				</Button>
			</form>
		{/if}
	</div>
</div>
