<script lang="ts">
	import { createEventDispatcher, tick } from 'svelte';
	import { labelColor, labelStyle } from '$lib/utils/labelColor';

	export let labels: string[] = [];
	export let suggestions: string[] = [];
	export let placeholder = 'Type a label, press space to add';
	export let disabled = false;
	export let allowCreate = true;
	/** name -> custom #rrggbb color, overriding the palette */
	export let colorMap: Record<string, string> = {};

	const dispatch = createEventDispatcher<{ change: string[] }>();

	let draft = '';
	let open = false;
	let highlight = -1;

	let current: string[] = [];
	$: if (labels !== current) current = [...labels];

	$: filtered = suggestions
		.filter((s) => !current.includes(s))
		.filter((s) => draft.trim() === '' || s.includes(draft.trim().toLowerCase()))
		.slice(0, 8);

	function emit() {
		current = [...current];
		labels = current;
		dispatch('change', [...current]);
	}

	function normalize(raw: string): string | null {
		const norm = raw.trim().toLowerCase().replace(/\s+/g, ' ');
		if (norm === '') return null;
		if (!/^[a-z0-9][a-z0-9 ._-]{0,31}$/.test(norm)) return '';
		return norm;
	}

	function add(raw: string) {
		if (!allowCreate) return;
		const norm = normalize(raw);
		if (norm === null) return;
		if (norm === '') return;
		if (!current.includes(norm)) {
			current = [...current, norm];
			emit();
		}
		draft = '';
		open = false;
		highlight = -1;
	}

	function remove(name: string) {
		if (disabled) return;
		current = current.filter((l) => l !== name);
		emit();
	}

	function onKeydown(e: KeyboardEvent) {
		if (open && filtered.length > 0) {
			if (e.key === 'ArrowDown') {
				e.preventDefault();
				highlight = (highlight + 1) % filtered.length;
				return;
			}
			if (e.key === 'ArrowUp') {
				e.preventDefault();
				highlight = (highlight - 1 + filtered.length) % filtered.length;
				return;
			}
			if (e.key === 'Enter' && highlight >= 0) {
				e.preventDefault();
				add(filtered[highlight]);
				return;
			}
			if (e.key === 'Escape') {
				open = false;
				return;
			}
		}
		if (e.key === ' ' || e.key === 'Enter' || e.key === ',') {
			e.preventDefault();
			add(draft);
		} else if (e.key === 'Backspace' && draft === '' && current.length > 0) {
			current = current.slice(0, -1);
			emit();
		}
	}

	async function onInput() {
		// `filtered` is reactive; let it catch up with the new draft first.
		await tick();
		open = filtered.length > 0 && draft.trim() !== '';
		highlight = open ? 0 : -1;
	}

	function pickSuggestion(name: string) {
		current = [...current, name];
		emit();
		draft = '';
		open = false;
		highlight = -1;
	}

	let container: HTMLDivElement;
	function onBlur(e: FocusEvent) {
		const related = e.relatedTarget as Node | null;
		setTimeout(() => {
			if (container && related && container.contains(related)) return;
			if (container && document.activeElement && container.contains(document.activeElement)) return;
			open = false;
		}, 120);
	}
</script>

<div class="relative" bind:this={container}>
	<div
		class="flex flex-wrap items-center gap-1.5 min-h-10 w-full px-2 py-1.5 border rounded-md
			focus-within:ring-2 focus-within:ring-blue-500 bg-white dark:bg-gray-800
			border-gray-300 dark:border-gray-600 {disabled ? 'opacity-50 cursor-not-allowed' : ''}"
	>
		{#each current as label (label)}
			<span class="inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-xs font-medium {labelColor(label, colorMap[label])}" style={labelStyle(colorMap[label])}>
				{label}
				{#if !disabled}
					<button
						type="button"
						class="hover:text-blue-600 dark:hover:text-blue-300"
						aria-label="Remove label {label}"
						on:click={() => remove(label)}
					>
						&times;
					</button>
				{/if}
			</span>
		{/each}
		{#if !disabled}
			<input
				type="text"
				bind:value={draft}
				{placeholder}
				autocomplete="off"
				on:keydown={onKeydown}
				on:input={onInput}
				on:blur={onBlur}
				class="flex-1 min-w-[8rem] bg-transparent text-sm text-gray-900 dark:text-gray-100 focus:outline-none"
			/>
		{/if}
	</div>
	{#if open}
		<ul
			class="absolute z-20 mt-1 w-full max-h-64 overflow-auto rounded-md border border-gray-200
				dark:border-gray-700 bg-white dark:bg-gray-800 shadow-lg"
			role="listbox"
		>
			{#each filtered as name, i (name)}
				<li>
					<button
						type="button"
						class="w-full px-3 py-2 text-left text-sm hover:bg-gray-100 dark:hover:bg-gray-700
							{i === highlight ? 'bg-gray-100 dark:bg-gray-700' : ''} text-gray-900 dark:text-gray-100"
						role="option"
						aria-selected={i === highlight}
						on:mousedown|preventDefault={() => pickSuggestion(name)}
					>
						<span class="inline-block w-2 h-2 rounded-full mr-2 {labelColor(name, colorMap[name])}" style={labelStyle(colorMap[name])}></span>{name}
					</button>
				</li>
			{/each}
		</ul>
	{/if}
</div>
