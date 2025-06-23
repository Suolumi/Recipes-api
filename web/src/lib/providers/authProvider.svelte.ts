import {getContext, setContext} from "svelte";
import {browser, dev} from "$app/environment";
import {fetcher as fetch, setFetcherContext} from "$lib/utils/fetcher.js";
import type {User} from "$lib/utils/types";

interface AuthTokens {
    access_token: string;
    expires_at: string;
    expires_in: number;
    refresh_token: string;
    refresh_token_expires_at: string;
    refresh_token_expires_in: number;
}

const contextKey = Symbol('authContext');

const isAuthTokens = (obj: any): obj is AuthTokens => {
    return obj && typeof obj.access_token === 'string' &&
        typeof obj.expires_at === 'string' &&
        typeof obj.expires_in === 'number' &&
        typeof obj.refresh_token === 'string' &&
        typeof obj.refresh_token_expires_at === 'string' &&
        typeof obj.refresh_token_expires_in === 'number';
}

export class AuthContext {
    tokens = $state<AuthTokens>()
    user = $state<User>()

    constructor() {
        if (!browser) return;

        try {
            const tokens = JSON.parse(localStorage.getItem('tokens') || '{}');

            if (isAuthTokens(tokens)) {
                this.tokens = tokens;
                setFetcherContext(this);
                void this.fetchProfile();
            } else {
                dev && console.warn('No valid auth tokens in localStorage.');
            }
        } catch (error) {
            dev && console.error('Token parse error:', error instanceof Error ? error.message : error);
        }
    }

    async fetchProfile(): Promise<void> {
        const response = await fetch('/users/me', undefined, true)

        if (response.ok) {
            this.user = await response.json();
        }
    }

    clear(): void {
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
