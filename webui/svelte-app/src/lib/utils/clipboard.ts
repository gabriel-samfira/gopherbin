/**
 * Copies text to clipboard with fallback for non-secure contexts
 */
export async function copyToClipboard(text: string): Promise<void> {
	// Try modern Clipboard API first (requires HTTPS or localhost)
	if (navigator.clipboard && navigator.clipboard.writeText) {
		try {
			await navigator.clipboard.writeText(text);
			return;
		} catch (err) {
			// Fall through to fallback
			console.warn('Clipboard API failed, using fallback:', err);
		}
	}

	// Fallback for non-secure contexts or when Clipboard API fails
	const textArea = document.createElement('textarea');
	textArea.value = text;
	textArea.style.position = 'fixed';
	textArea.style.left = '-999999px';
	textArea.style.top = '-999999px';
	document.body.appendChild(textArea);
	textArea.focus();
	textArea.select();

	try {
		document.execCommand('copy');
	} catch (err) {
		console.error('Failed to copy text:', err);
		throw new Error('Failed to copy to clipboard');
	} finally {
		document.body.removeChild(textArea);
	}
}
