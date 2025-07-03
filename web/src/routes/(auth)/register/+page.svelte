<script lang="ts">
	import { Field, Control, Label, FieldErrors } from 'formsnap';
	import { superForm } from 'sveltekit-superforms';
	import { zodClient } from 'sveltekit-superforms/adapters';
	import { registerSchema } from '$lib/utils/schemas';
	import Loader from '@lucide/svelte/icons/loader';
	import { toaster } from '$lib/utils/toaster';

	const { data } = $props();
	// todo: add better error handling
	const form = superForm(data.form, {
		validators: zodClient(registerSchema),
		onError: ({ result }) => {
			toaster[result.type]({
				title: result.error.message
			});
		}
	});
	const { form: formData, enhance, delayed, message } = form;
</script>

<form method="POST" use:enhance class="mx-auto w-full max-w-sm space-y-6">
	<h1 class="h1">Register</h1>

	<Field {form} name="username">
		<Control>
			{#snippet children({ props })}
				<Label class="label">
					<span class="label-text">Username</span>
					<input class="input" {...props} bind:value={$formData.username} />
				</Label>
			{/snippet}
		</Control>
		<FieldErrors />
	</Field>

	<Field {form} name="email">
		<Control>
			{#snippet children({ props })}
				<Label class="label">
					<span class="label-text">Email</span>
					<input class="input" {...props} type="email" bind:value={$formData.email} />
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

	<Field {form} name="confirm">
		<Control>
			{#snippet children({ props })}
				<Label class="label">
					<span class="label-text">Confirm password</span>
					<input class="input" {...props} type="password" bind:value={$formData.confirm} />
				</Label>
			{/snippet}
		</Control>
		<FieldErrors />
	</Field>

	<button disabled={$delayed} type="submit" class="btn preset-filled w-full">
		{#if $delayed}
			<Loader class="animate-spin" size={28} />
		{:else}
			Register
		{/if}
	</button>

	{#if $message}
		<span>
			{$message.text}
		</span>
	{/if}
</form>
