<script lang="ts">
	import '../app.css';
	import Header from '$lib/components/layout/Header.svelte';
	import Toast from '$lib/components/ui/Toast.svelte';
	import { auth } from '$lib/stores/auth';
	import { theme } from '$lib/stores/theme';
	import { onMount } from 'svelte';

	// Initialize auth and theme on mount
	onMount(() => {
		auth.checkAuth();
		// Theme is already initialized in its store
	});
</script>

<div class="min-h-screen bg-gray-50 dark:bg-gray-900">
	<Header />
	<main class="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-8">
		<slot />
	</main>
	<Toast />
</div>
