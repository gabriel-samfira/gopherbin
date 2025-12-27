<script lang="ts">
	import { auth } from '$lib/stores/auth';
	import ThemeToggle from '$lib/components/ui/ThemeToggle.svelte';
	import { goto } from '$app/navigation';
	import { Menu, X } from 'lucide-svelte';

	let mobileMenuOpen = false;

	function handleLogout() {
		goto('/logout');
		mobileMenuOpen = false;
	}

	function closeMobileMenu() {
		mobileMenuOpen = false;
	}
</script>

<header class="bg-white dark:bg-gray-800 shadow-sm border-b border-gray-200 dark:border-gray-700">
	<div class="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8">
		<div class="flex justify-between items-center h-16">
			<!-- Logo -->
			<div class="flex items-center gap-2 sm:gap-3">
				<a href="/" class="flex items-center gap-2">
					<img src="/logo.svg" alt="GopherBin" class="h-8 sm:h-10 w-auto" />
					<span class="text-xl sm:text-2xl font-bold text-blue-600 dark:text-blue-500">
						GopherBin
					</span>
				</a>
			</div>

			<!-- Desktop Navigation -->
			<nav class="hidden md:flex items-center gap-4">
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

			<!-- Mobile menu button & theme toggle -->
			<div class="flex md:hidden items-center gap-2">
				<ThemeToggle />
				<button
					on:click={() => (mobileMenuOpen = !mobileMenuOpen)}
					class="p-2 rounded-md text-gray-700 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700"
					aria-label="Toggle menu"
				>
					{#if mobileMenuOpen}
						<X class="w-6 h-6" />
					{:else}
						<Menu class="w-6 h-6" />
					{/if}
				</button>
			</div>
		</div>
	</div>

	<!-- Mobile Navigation -->
	{#if mobileMenuOpen}
		<div class="md:hidden border-t border-gray-200 dark:border-gray-700">
			<nav class="px-4 py-4 space-y-3">
				{#if $auth.isAuthenticated}
					<a
						href="/"
						on:click={closeMobileMenu}
						class="block py-2 text-gray-700 dark:text-gray-300 hover:text-blue-600 dark:hover:text-blue-500"
					>
						New Paste
					</a>
					<a
						href="/p"
						on:click={closeMobileMenu}
						class="block py-2 text-gray-700 dark:text-gray-300 hover:text-blue-600 dark:hover:text-blue-500"
					>
						My Pastes
					</a>
					{#if $auth.isAdmin}
						<a
							href="/admin/users"
							on:click={closeMobileMenu}
							class="block py-2 text-gray-700 dark:text-gray-300 hover:text-blue-600 dark:hover:text-blue-500"
						>
							Admin
						</a>
					{/if}
					<button
						on:click={handleLogout}
						class="block w-full text-left py-2 text-gray-700 dark:text-gray-300 hover:text-blue-600 dark:hover:text-blue-500"
					>
						Logout
					</button>
				{:else}
					<a
						href="/login"
						on:click={closeMobileMenu}
						class="block py-2 text-gray-700 dark:text-gray-300 hover:text-blue-600 dark:hover:text-blue-500"
					>
						Login
					</a>
				{/if}
			</nav>
		</div>
	{/if}
</header>
