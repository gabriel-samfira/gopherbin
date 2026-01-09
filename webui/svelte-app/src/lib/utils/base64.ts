/**
 * Encodes a UTF-8 string to base64
 */
export function encodeBase64(str: string): string {
	const utf8Bytes = new TextEncoder().encode(str);
	return btoa(String.fromCharCode(...utf8Bytes));
}

/**
 * Decodes a base64 string to UTF-8
 */
export function decodeBase64(base64: string): string {
	const binaryString = atob(base64);
	const bytes = new Uint8Array(binaryString.length);
	for (let i = 0; i < binaryString.length; i++) {
		bytes[i] = binaryString.charCodeAt(i);
	}
	return new TextDecoder().decode(bytes);
}
