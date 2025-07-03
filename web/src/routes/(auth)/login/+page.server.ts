import { message, setError, superValidate } from 'sveltekit-superforms';
import { zod4 } from 'sveltekit-superforms/adapters';
import { loginSchema } from '$lib/utils/schemas';
import { fail } from '@sveltejs/kit';
import { redirect } from 'sveltekit-flash-message/server';

export const load = async () => {
	return { form: await superValidate(zod4(loginSchema)) };
};

export const actions = {
	default: async (event) => {
		const form = await superValidate(event.request, zod4(loginSchema));

		if (!form.valid) {
			return fail(400, { form });
		}

		const response = await fetch(import.meta.env.VITE_API_BASE + '/login', {
			method: 'POST',
			headers: {
				'Content-Type': 'application/json'
			},
			body: JSON.stringify(form.data)
		});
		const data = await response.json();

		if (!response.ok) {
			if (data.error && data.error === 'Incorrect username or password') {
				setError(form, 'id', 'todo: paraglide error');
				return setError(form, 'password', 'todo: paraglide error');
			}
			return message(form, 'An unexpected error occurred. Please try again later.', {
				status: response.status as 400 | 404 | 500
			});
		}

		redirect(
			'/',
			{ type: 'success', message: 'You have successfully logged in.', tokens: data },
			event
		);
	}
};
