<script lang="ts">
	import '../app.css';
	import { initFlash } from 'sveltekit-flash-message';
	import { page } from '$app/state';
	import { Toaster } from '@skeletonlabs/skeleton-svelte';
	import { toaster } from '$lib/utils/toaster';
	import { createAuthContext } from '$lib/providers/authProvider.svelte';
	import { untrack } from 'svelte';
	import { setFetcherContext } from '$lib/utils/fetcher';

	let { children } = $props();
	const flash = initFlash(page);
	const authCtx = createAuthContext();

	$effect(() => {
		if ($flash) {
			toaster[$flash.type]({
				title: $flash.message
			});

			if ($flash.tokens) {
				untrack(() => {
					setFetcherContext(authCtx);
					authCtx.setTokens($flash.tokens!);
				});
			}
		}
	});
</script>

<Toaster {toaster}></Toaster>

{@render children()}
