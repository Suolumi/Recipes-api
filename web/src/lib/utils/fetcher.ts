import {AuthContext} from "$lib/providers/authProvider.svelte";

const API_BASE = 'http://localhost:8080';
let authContext: AuthContext | null = null;
let isRefreshing = false;
let refreshPromise: Promise<boolean> | null = null;

async function refreshToken(refreshToken: string): Promise<boolean> {
    if (!refreshPromise) {
        isRefreshing = true;
        refreshPromise = fetch(`${API_BASE}/refresh`, {
            method: 'POST',
            headers: {
                'Authorization': `Bearer ${refreshToken}`,
            }
        })
            .then(res => res.ok)
            .catch(() => false)
            .finally(() => {
                isRefreshing = false;
                refreshPromise = null;
            });
    }
    return refreshPromise;
}

export async function fetcher(input: RequestInfo, init: RequestInit = {}, isProtected = false): Promise<Response> {
    if (isProtected) {
        if (!authContext || !authContext.tokens) {
            throw new Error('Auth context is not set. Please set the auth context using setFetcherContext.');
        }

        const baseHeaders = init.headers instanceof Headers
            ? init.headers
            : new Headers(init?.headers);

        baseHeaders.set('Authorization', `Bearer ${authContext.tokens.access_token}`);
        init = {...init, headers: baseHeaders};
    }

    let url = typeof input === 'string' && !input.startsWith('http') ? `${API_BASE}${input}` : input;
    let response = await fetch(url, {...init});

    if (response.status !== 401 || !isProtected) return response;

    const refreshed = isRefreshing && refreshPromise
        ? await refreshPromise
        : await refreshToken(authContext!.tokens!.refresh_token);

    if (!refreshed) {
        authContext!.clear();
        return response;
    }

    return fetch(url, {...init});
}

export const setFetcherContext = (context: AuthContext) => {
    authContext = context;
}
