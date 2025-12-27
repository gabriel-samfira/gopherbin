<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { auth } from '$lib/stores/auth';
	import { logout } from '$lib/api/auth';

	onMount(async () => {
		const token = $auth.token;

		// Call logout API if we have a token
		if (token) {
			try {
				await logout(token);
			} catch (err) {
				// Ignore errors, we're logging out anyway
				console.error('Logout API error:', err);
			}
		}

		// Clear local auth state
		auth.logout();

		// Redirect to login
		goto('/login');
	});
</script>

<svelte:head>
	<title>Logging out - GopherBin</title>
</svelte:head>

<div class="flex justify-center items-center min-h-[60vh]">
	<div class="text-center">
		<div class="animate-spin rounded-full h-12 w-12 border-b-2 border-blue-600 dark:border-blue-500 mx-auto"></div>
		<p class="mt-4 text-gray-600 dark:text-gray-400">Logging out...</p>
	</div>
</div>
