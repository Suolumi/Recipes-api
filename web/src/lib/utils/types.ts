export interface AuthTokens {
	access_token: string;
	expires_at: string;
	expires_in: number;
	refresh_token: string;
	refresh_token_expires_at: string;
	refresh_token_expires_in: number;
}

export const isAuthTokens = (obj: unknown): obj is AuthTokens => {
	if (typeof obj !== 'object' || obj === null) return false;
	const o = obj as Record<string, unknown>;
	return (
		typeof o.access_token === 'string' &&
		typeof o.expires_at === 'string' &&
		typeof o.expires_in === 'number' &&
		typeof o.refresh_token === 'string' &&
		typeof o.refresh_token_expires_at === 'string' &&
		typeof o.refresh_token_expires_in === 'number'
	);
};

export type User = {
	id: string;
	email: string;
	username: string;
};
