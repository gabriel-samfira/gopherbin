<script lang="ts">
	export let checked = false;
	export let label = '';
	export let disabled = false;
</script>

<label class="flex items-center gap-2 cursor-pointer">
	{#if label}
		<span class="text-sm font-medium text-gray-700 dark:text-gray-300">{label}</span>
	{/if}
	<button
		type="button"
		role="switch"
		aria-checked={checked}
		aria-label={label || 'Toggle'}
		{disabled}
		on:click={() => !disabled && (checked = !checked)}
		class="relative inline-flex h-6 w-11 items-center rounded-full transition-colors focus:outline-none focus:ring-2 focus:ring-blue-500 focus:ring-offset-2 disabled:opacity-50 disabled:cursor-not-allowed {checked
			? 'bg-blue-600'
			: 'bg-gray-200 dark:bg-gray-700'}"
	>
		<span
			class="inline-block h-4 w-4 transform rounded-full bg-white transition-transform {checked
				? 'translate-x-6'
				: 'translate-x-1'}"
		></span>
	</button>
</label>
