// Minimal stand-ins for the SvelteKit ambient modules. The app code under
// test runs unmodified against them; they only replace framework plumbing,
// never application logic or network calls.

export const browser = true;
export const dev = false;
export const building = false;
export const version = 'integration-tests';
