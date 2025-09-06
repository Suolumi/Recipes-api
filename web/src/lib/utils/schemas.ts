import { z } from 'zod/v4';

export const loginSchema = z.object({
	id: z.string(),
	password: z.string()
});

export const registerSchema = z
	.object({
		username: z.string().max(32),
		email: z.email(),
		password: z
			.string()
			.min(10)
			.refine((val) => {
				return /[0-9]/.test(val) && /[!@#$%^&*(),.?":{}|<>]/.test(val);
			}),
		confirm: z.string()
	})
	.refine(
		(val) => {
			return val.password === val.confirm;
		},
		{ message: '(todo: paraglide error) Passwords do not match', path: ['confirm'] }
	);
