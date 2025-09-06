<script lang="ts">
	import { Field, Control, Label, FieldErrors } from 'formsnap';
	import { superForm } from 'sveltekit-superforms';
	import { zodClient } from 'sveltekit-superforms/adapters';
	import { loginSchema } from '$lib/utils/schemas';
	import Loader from '@lucide/svelte/icons/loader';
	import { toaster } from '$lib/utils/toaster';

	const { data } = $props();
	// todo: add better error handling
	const form = superForm(data.form, {
		validators: zodClient(loginSchema),
		onError: ({ result }) => {
			toaster[result.type]({
				title: result.error.message
			});
		}
	});
	const { form: formData, enhance, delayed } = form;
</script>

<form method="POST" use:enhance class="mx-auto w-full max-w-sm space-y-6">
	<h1 class="h1">Login</h1>

	<Field {form} name="id">
		<Control>
			{#snippet children({ props })}
				<Label class="label">
					<span class="label-text">Email or Username</span>
					<input class="input" {...props} bind:value={$formData.id} />
				</Label>
			{/snippet}
		</Control>
		<FieldErrors />
	</Field>

	<Field {form} name="password">
		<Control>
			{#snippet children({ props })}
				<Label class="label">
					<span class="label-text">Password</span>
					<input class="input" {...props} type="password" bind:value={$formData.password} />
				</Label>
			{/snippet}
		</Control>
		<FieldErrors />
	</Field>

	<button disabled={$delayed} type="submit" class="btn preset-filled w-full">
		{#if $delayed}
			<Loader class="animate-spin" size={28} />
		{:else}
			Login
		{/if}
	</button>
</form>
