<script lang="ts">
	export let variant: 'primary' | 'secondary' | 'success' | 'danger' = 'primary';
	export let disabled = false;
	export let type: 'button' | 'submit' | 'reset' = 'button';
	let className = '';
	export { className as class };

	const variants = {
		primary: 'bg-blue-600 hover:bg-blue-700 text-white dark:bg-blue-500 dark:hover:bg-blue-600',
		secondary: 'bg-gray-600 hover:bg-gray-700 text-white dark:bg-gray-500 dark:hover:bg-gray-600',
		success: 'bg-green-600 hover:bg-green-700 text-white dark:bg-green-500 dark:hover:bg-green-600',
		danger: 'bg-red-600 hover:bg-red-700 text-white dark:bg-red-500 dark:hover:bg-red-600'
	};
</script>

<button
	{type}
	{disabled}
	on:click
	class="px-4 py-2 rounded-md font-medium transition-colors disabled:opacity-50 disabled:cursor-not-allowed {variants[variant]} {className}"
>
	<slot />
</button>
