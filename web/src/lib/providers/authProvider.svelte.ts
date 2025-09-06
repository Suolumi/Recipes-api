import { browser, dev } from '$app/environment';
import { fetcher as fetch, setFetcherContext } from '$lib/utils/fetcher.js';
import { type AuthTokens, type User, isAuthTokens } from '$lib/utils/types';
import { setContext, getContext } from 'svelte';

const contextKey = Symbol('authContext');

export class AuthContext {
	tokens = $state<AuthTokens>();
	user = $state<User>();

	constructor() {
		if (!browser) return;

		try {
			const tokens = JSON.parse(localStorage.getItem('tokens') || '{}');

			if (isAuthTokens(tokens)) {
				this.tokens = tokens;
				setFetcherContext(this);
				void this.fetchProfile();
			} else {
				if (dev) console.warn('No valid auth tokens in localStorage.');
			}
		} catch (error) {
			if (dev) console.error('Token parse error:', error instanceof Error ? error.message : error);
		}
	}

	async fetchProfile() {
		const response = await fetch('/users/me', undefined, true);

		if (response.ok) {
			if (dev) console.info('User profile fetched successfully.');
			this.user = await response.json();
		}
	}

	setTokens(tokens: AuthTokens) {
		this.clear();
		this.tokens = tokens;
		localStorage.setItem('tokens', JSON.stringify(tokens));
		void this.fetchProfile();
	}

	clear() {
		this.tokens = undefined;
		this.user = undefined;

		if (browser) {
			localStorage.removeItem('tokens');
		}
	}
}

export const createAuthContext = () => {
	return setContext(contextKey, new AuthContext());
};

export const getAuthContext = () => {
	return getContext<AuthContext>(contextKey);
};
