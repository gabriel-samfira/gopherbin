<script lang="ts">
	import { createEventDispatcher } from 'svelte';
	import { searchUsers } from '$lib/api/users';
	import type { UserSearchResult } from '$lib/types/user';

	export let token: string;
	/** Team whose members (and the caller) should be excluded from results. */
	export let team = '';
	export let placeholder = 'Username or email';
	export let disabled = false;
	export let autofocus = false;
	export let value = '';

	const dispatch = createEventDispatcher<{ select: UserSearchResult }>();

	let results: UserSearchResult[] = [];
	let open = false;
	let loading = false;
	let highlight = -1;
	let timer: ReturnType<typeof setTimeout>;
	let container: HTMLDivElement;

	async function fetchResults() {
		if (value.trim().length < 2) {
			results = [];
			open = false;
			return;
		}
		loading = true;
		try {
			results = await searchUsers(value.trim(), team, token);
			open = results.length > 0;
			highlight = results.length > 0 ? 0 : -1;
		} catch {
			results = [];
			open = false;
		} finally {
			loading = false;
		}
	}

	function onInput() {
		clearTimeout(timer);
		timer = setTimeout(fetchResults, 250);
	}

	function pick(user: UserSearchResult) {
		value = user.username;
		open = false;
		results = [];
		dispatch('select', user);
	}

	function onKeydown(e: KeyboardEvent) {
		if (!open) return;
		if (e.key === 'ArrowDown') {
			e.preventDefault();
			highlight = (highlight + 1) % results.length;
		} else if (e.key === 'ArrowUp') {
			e.preventDefault();
			highlight = (highlight - 1 + results.length) % results.length;
		} else if (e.key === 'Enter' && highlight >= 0) {
			e.preventDefault();
			pick(results[highlight]);
		} else if (e.key === 'Escape') {
			open = false;
		}
	}

	function onBlur(e: FocusEvent) {
		// Give clicks on dropdown rows a chance to land before closing.
		const related = e.relatedTarget as Node | null;
		setTimeout(() => {
			if (container && related && container.contains(related)) return;
			if (container && document.activeElement && container.contains(document.activeElement)) return;
			open = false;
		}, 120);
	}
</script>

<div class="relative flex-1" bind:this={container}>
	<!-- svelte-ignore a11y-autofocus -->
	<input
		type="text"
		bind:value={value}
		{placeholder}
		{disabled}
		{autofocus}
		autocomplete="off"
		on:input={onInput}
		on:keydown={onKeydown}
		on:focus={() => (open = results.length > 0)}
		on:blur={onBlur}
		class="w-full h-10 px-3 text-sm border rounded-md focus:outline-none focus:ring-2 focus:ring-blue-500
			bg-white dark:bg-gray-800 text-gray-900 dark:text-gray-100
			border-gray-300 dark:border-gray-600
			disabled:opacity-50 disabled:cursor-not-allowed"
	/>
	{#if loading}
		<span class="absolute right-3 top-1/2 -translate-y-1/2 text-xs text-gray-400">…</span>
	{:else if open}
		<ul
			class="absolute z-20 mt-1 w-full max-h-64 overflow-auto rounded-md border border-gray-200
				dark:border-gray-700 bg-white dark:bg-gray-800 shadow-lg"
			role="listbox"
		>
			{#each results as user, i (user.id)}
				<li>
					<button
						type="button"
						class="w-full px-3 py-2 text-left text-sm flex items-center gap-2 hover:bg-gray-100
							dark:hover:bg-gray-700 {i === highlight ? 'bg-gray-100 dark:bg-gray-700' : ''}"
						role="option"
						aria-selected={i === highlight}
						on:mousedown|preventDefault={() => pick(user)}
					>
						<span class="font-medium text-gray-900 dark:text-gray-100">{user.full_name}</span>
						<span class="text-gray-500 dark:text-gray-400">@{user.username}</span>
					</button>
				</li>
			{/each}
		</ul>
	{/if}
</div>
