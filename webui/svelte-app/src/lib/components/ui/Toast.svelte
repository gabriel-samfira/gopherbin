<script lang="ts">
	import { toast } from '$lib/stores/toast';
	import { X, CheckCircle, AlertCircle, Info, AlertTriangle } from 'lucide-svelte';

	$: toasts = $toast;

	const icons = {
		success: CheckCircle,
		error: AlertCircle,
		info: Info,
		warning: AlertTriangle
	};

	const colors = {
		success: 'bg-green-50 dark:bg-green-900/20 text-green-800 dark:text-green-200 border-green-200 dark:border-green-800',
		error: 'bg-red-50 dark:bg-red-900/20 text-red-800 dark:text-red-200 border-red-200 dark:border-red-800',
		info: 'bg-blue-50 dark:bg-blue-900/20 text-blue-800 dark:text-blue-200 border-blue-200 dark:border-blue-800',
		warning: 'bg-yellow-50 dark:bg-yellow-900/20 text-yellow-800 dark:text-yellow-200 border-yellow-200 dark:border-yellow-800'
	};
</script>

<div class="fixed top-4 right-4 z-50 flex flex-col gap-2 max-w-md">
	{#each toasts as t (t.id)}
		<div
			class="flex items-center gap-3 p-4 rounded-lg border shadow-lg animate-slide-in {colors[t.type]}"
			role="alert"
		>
			<svelte:component this={icons[t.type]} class="w-5 h-5 flex-shrink-0" />
			<p class="flex-1 text-sm font-medium">{t.message}</p>
			<button
				on:click={() => toast.dismiss(t.id)}
				class="flex-shrink-0 hover:opacity-70 transition-opacity"
				aria-label="Dismiss"
			>
				<X class="w-4 h-4" />
			</button>
		</div>
	{/each}
</div>

<style>
	@keyframes slide-in {
		from {
			transform: translateX(100%);
			opacity: 0;
		}
		to {
			transform: translateX(0);
			opacity: 1;
		}
	}

	.animate-slide-in {
		animation: slide-in 0.3s ease-out;
	}
</style>