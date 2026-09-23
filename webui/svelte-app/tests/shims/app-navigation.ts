// Navigation shim: goto() is recorded instead of moving the browser, so
// tests can assert where a component tried to navigate.

export const navigations: string[] = [];

export async function goto(url: string): Promise<boolean> {
	navigations.push(url);
	return true;
}

export function beforeNavigate(): void {
	/* no-op */
}

export function afterNavigate(): void {
	/* no-op */
}

export function beforeLeave(): void {
	/* no-op */
}
