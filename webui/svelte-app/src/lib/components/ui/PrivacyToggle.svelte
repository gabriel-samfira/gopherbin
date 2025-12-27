<script lang="ts">
	export let isPublic = false;
	export let onChange: (value: boolean) => void = () => {};

	function toggle() {
		isPublic = !isPublic;
		onChange(isPublic);
	}
</script>

<div class="flex items-center gap-2">
	<span class="text-sm text-gray-700 dark:text-gray-300">
		{isPublic ? 'Public' : 'Private'}
	</span>
	<button
		on:click={toggle}
		class="relative inline-flex h-6 w-11 items-center rounded-full transition-colors
			{isPublic ? 'bg-green-600' : 'bg-gray-300 dark:bg-gray-600'}"
		role="switch"
		aria-checked={isPublic}
		aria-label="Toggle privacy"
	>
		<span
			class="inline-block h-4 w-4 transform rounded-full bg-white transition-transform
				{isPublic ? 'translate-x-6' : 'translate-x-1'}"
		></span>
	</button>
</div>
