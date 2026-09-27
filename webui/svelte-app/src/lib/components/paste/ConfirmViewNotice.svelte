<script lang="ts">
	import Button from '$lib/components/ui/Button.svelte';
	import { Eye } from 'lucide-svelte';

	// Shown instead of a limited-access paste until the viewer agrees to use
	// up one of its views; the page reloads the paste with consume=true on
	// the confirm event.
	export let busy = false;
</script>

<div
	class="max-w-xl mx-auto p-6 bg-amber-50 dark:bg-amber-900/30 border border-amber-200 dark:border-amber-800 rounded-lg space-y-4"
>
	<h1 class="text-xl font-bold text-gray-900 dark:text-gray-100">This paste has limited views</h1>
	<p class="text-sm text-gray-700 dark:text-gray-300">
		Opening it uses up one of its views. Once the last view is used, the paste is deleted for
		everyone.
	</p>
	<Button on:click variant="primary" disabled={busy}>
		<span class="inline-flex items-center gap-2">
			<Eye class="w-4 h-4" />
			{busy ? 'Opening...' : 'View paste'}
		</span>
	</Button>
</div>
