const PALETTE = [
	'bg-red-100 text-red-800 dark:bg-red-900 dark:text-red-200',
	'bg-orange-100 text-orange-800 dark:bg-orange-900 dark:text-orange-200',
	'bg-amber-100 text-amber-800 dark:bg-amber-900 dark:text-amber-200',
	'bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-200',
	'bg-teal-100 text-teal-800 dark:bg-teal-900 dark:text-teal-200',
	'bg-blue-100 text-blue-800 dark:bg-blue-900 dark:text-blue-200',
	'bg-indigo-100 text-indigo-800 dark:bg-indigo-900 dark:text-indigo-200',
	'bg-pink-100 text-pink-800 dark:bg-pink-900 dark:text-pink-200'
];

/**
 * Deterministic badge color for a label name: the same label renders with
 * the same color everywhere for every user.
 */
const HEX_RE = /^#[0-9a-fA-F]{6}$/;

function isHex(custom?: string): boolean {
	return !!custom && HEX_RE.test(custom);
}

/**
 * Deterministic badge color for a label name: the same label renders with
 * the same color everywhere for every user. Returns a Tailwind class when
 * the label uses the automatic palette; an empty string when a custom color
 * is set (render it with `labelStyle` instead).
 */
export function labelColor(name: string, custom?: string): string {
	if (isHex(custom)) return '';
	let h = 2166136261;
	for (let i = 0; i < name.length; i++) {
		h ^= name.charCodeAt(i);
		h = Math.imul(h, 16777619);
	}
	return PALETTE[Math.abs(h) % PALETTE.length];
}

/**
 * Inline style for badges of a custom-colored label, with a readable
 * foreground picked from the color's luminance. Empty for palette labels.
 */
export function labelStyle(custom?: string): string {
	if (!isHex(custom)) return '';
	const r = parseInt(custom!.slice(1, 3), 16);
	const g = parseInt(custom!.slice(3, 5), 16);
	const b = parseInt(custom!.slice(5, 7), 16);
	const lum = (0.299 * r + 0.587 * g + 0.114 * b) / 255;
	return `background-color:${custom};color:${lum > 0.6 ? '#111827' : '#f9fafb'}`;
}
