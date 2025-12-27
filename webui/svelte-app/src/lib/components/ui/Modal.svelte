<script lang="ts">
	export let show = false;
	export let onClose: () => void = () => {};
</script>

{#if show}
	<div
		class="fixed inset-0 flex items-center justify-center z-50 p-2 sm:p-4"
		style="background-color: rgba(0, 0, 0, 0.3); backdrop-filter: blur(4px);"
		on:click={onClose}
		on:keydown={(e) => e.key === 'Escape' && onClose()}
		role="button"
		tabindex="0"
	>
		<div
			class="bg-white dark:bg-gray-800 rounded-lg max-w-2xl w-full max-h-[90vh] overflow-y-auto p-4 sm:p-6 shadow-xl"
			on:click|stopPropagation
			on:keydown|stopPropagation
			role="dialog"
			tabindex="-1"
		>
			<slot />
		</div>
	</div>
{/if}
