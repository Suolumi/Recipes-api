import {getContext, setContext} from "svelte";
import {browser, dev} from "$app/environment";

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

    constructor() {
        if (!browser) return;

        try {
            const tokens = JSON.parse(localStorage.getItem('tokens') || '{}');

            if (isAuthTokens(tokens)) {
                this.tokens = tokens;
                void this.authenticate()
            } else {
                dev && console.warn('User not authenticated');
            }
        } catch (error) {
            dev && console.error('Failed to parse tokens from localStorage:', error);
        }
    }

    async authenticate(): Promise<void> {
        // TODO: Implement
        throw new Error('authenticate method not implemented');
    }

    async fetchProfile(): Promise<void> {
        // TODO: Implement
        throw new Error('fetchProfile method not implemented');
    }
}

export const createAuthContext = () => {
    return setContext(contextKey, new AuthContext());
};

export const getAuthContext = () => {
    return getContext<AuthContext>(contextKey);
};
