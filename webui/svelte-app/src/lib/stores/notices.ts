import { writable } from 'svelte/store';

/**
 * Counter bumped whenever an action changed the current user's pending
 * notices (team invitations / ownership offers). The header notice bell
 * subscribes so it clears immediately instead of waiting for its next poll.
 */
export const noticesTick = writable(0);

export function refreshNotices(): void {
	noticesTick.update((n) => n + 1);
}
