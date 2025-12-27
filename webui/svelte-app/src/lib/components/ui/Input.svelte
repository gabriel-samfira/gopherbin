<script lang="ts">
	export let id: string | undefined = undefined;
	export let type = 'text';
	export let value = '';
	export let placeholder = '';
	export let disabled = false;
	export let invalid = false;
	export let readonly = false;
</script>

<input
	{id}
	{type}
	{placeholder}
	{disabled}
	{readonly}
	bind:value
	on:input
	on:change
	on:keydown
	class="w-full px-3 py-2 border rounded-md focus:outline-none focus:ring-2 focus:ring-blue-500
		bg-white dark:bg-gray-800 text-gray-900 dark:text-gray-100
		border-gray-300 dark:border-gray-600
		disabled:opacity-50 disabled:cursor-not-allowed
		read-only:opacity-75 read-only:cursor-default
		{invalid ? 'border-red-500 focus:ring-red-500' : ''}"
/>
