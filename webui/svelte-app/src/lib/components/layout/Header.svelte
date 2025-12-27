<script lang="ts">
	import { auth } from '$lib/stores/auth';
	import ThemeToggle from '$lib/components/ui/ThemeToggle.svelte';
	import { goto } from '$app/navigation';

	function handleLogout() {
		goto('/logout');
	}
</script>

<header class="bg-white dark:bg-gray-800 shadow-sm border-b border-gray-200 dark:border-gray-700">
	<div class="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8">
		<div class="flex justify-between items-center h-16">
			<div class="flex items-center gap-3">
				<a href="/" class="flex items-center gap-2">
					<img src="/logo.svg" alt="GopherBin" class="h-10 w-auto" />
					<span class="text-2xl font-bold text-blue-600 dark:text-blue-500">GopherBin</span>
				</a>
			</div>

			<nav class="flex items-center gap-4">
				{#if $auth.isAuthenticated}
					<a
						href="/"
						class="text-gray-700 dark:text-gray-300 hover:text-blue-600 dark:hover:text-blue-500"
					>
						New Paste
					</a>
					<a
						href="/p"
						class="text-gray-700 dark:text-gray-300 hover:text-blue-600 dark:hover:text-blue-500"
					>
						My Pastes
					</a>
					{#if $auth.isAdmin}
						<a
							href="/admin/users"
							class="text-gray-700 dark:text-gray-300 hover:text-blue-600 dark:hover:text-blue-500"
						>
							Admin
						</a>
					{/if}
					<button
						on:click={handleLogout}
						class="text-gray-700 dark:text-gray-300 hover:text-blue-600 dark:hover:text-blue-500"
					>
						Logout
					</button>
				{:else}
					<a
						href="/login"
						class="text-gray-700 dark:text-gray-300 hover:text-blue-600 dark:hover:text-blue-500"
					>
						Login
					</a>
				{/if}
				<ThemeToggle />
			</nav>
		</div>
	</div>
</header>
