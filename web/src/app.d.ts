// See https://svelte.dev/docs/kit/types#app.d.ts
// for information about these interfaces
import type { AuthTokens } from '$lib/utils/types';

declare global {
	namespace App {
		// interface Error {}
		// interface Locals {}
		// interface PageState {}
		// interface Platform {}

		interface PageData {
			flash?: { type: 'success' | 'error'; message: string; tokens?: AuthTokens };
		}
	}
}

export {};
