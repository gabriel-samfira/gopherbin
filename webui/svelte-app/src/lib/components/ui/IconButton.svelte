<script lang="ts">
	export let title: string = '';
	export let disabled = false;
	export let variant: 'default' | 'danger' = 'default';

	const variants = {
		default:
			'text-gray-600 dark:text-gray-400 hover:text-blue-600 dark:hover:text-blue-500 hover:bg-gray-100 dark:hover:bg-gray-700',
		danger:
			'text-gray-600 dark:text-gray-400 hover:text-red-600 dark:hover:text-red-500 hover:bg-red-50 dark:hover:bg-red-900/20'
	};
</script>

<button
	{title}
	{disabled}
	on:click
	class="p-2 rounded-md transition-colors disabled:opacity-50 disabled:cursor-not-allowed {variants[variant]}"
>
	<slot />
</button>
