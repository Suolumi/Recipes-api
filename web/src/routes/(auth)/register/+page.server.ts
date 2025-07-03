import { message, setError, superValidate } from 'sveltekit-superforms';
import { zod4 } from 'sveltekit-superforms/adapters';
import { registerSchema } from '$lib/utils/schemas';
import { fail } from '@sveltejs/kit';
import { redirect } from 'sveltekit-flash-message/server';

export const load = async () => {
	return { form: await superValidate(zod4(registerSchema)) };
};

export const actions = {
	default: async (event) => {
		const form = await superValidate(event.request, zod4(registerSchema));

		if (!form.valid) {
			return fail(400, { form });
		}

		const response = await fetch(import.meta.env.VITE_API_BASE + '/register', {
			method: 'POST',
			headers: {
				'Content-Type': 'application/json'
			},
			body: JSON.stringify(form.data)
		});
		const data = await response.json();

		if (!response.ok) {
			if (data.error) {
				if (data.error === 'username is already taken') {
					return setError(form, 'username', 'todo: paraglide error');
				} else if (data.error === 'email is already taken') {
					return setError(form, 'email', 'todo: paraglide error');
				}
			}

			return message(form, 'An unexpected error occurred. Please try again later.', {
				status: response.status as 400 | 406 | 409 | 500
			});
		}

		throw redirect(
			'/login',
			{ type: 'success', message: 'Registration successful! You can now log in.' },
			event
		);
	}
};
