<script lang="ts">
	import { AppBar } from '@skeletonlabs/skeleton-svelte';
	import ArrowLeft from '@lucide/svelte/icons/arrow-left';
	import CircleUser from '@lucide/svelte/icons/circle-user';
	import { getAuthContext } from '$lib/providers/authProvider.svelte';
	import { goto } from '$app/navigation';

	const authCtx = getAuthContext();
</script>

<AppBar toolbarClasses="items-center">
	{#snippet lead()}
		<ArrowLeft />
	{/snippet}

	{#snippet trail()}
		{#if authCtx.user}
			<CircleUser size={34} />
		{:else}
			<button
				onclick={() => goto('/register')}
				type="button"
				class="btn btn-md preset-filled-secondary-500">Register</button
			>
			<button
				onclick={() => goto('/login')}
				type="button"
				class="btn btn-md preset-filled-primary-500">Login</button
			>
		{/if}
	{/snippet}
</AppBar>
