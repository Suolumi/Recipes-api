<script lang="ts">
	import '../../app.css';
    import {createAuthContext} from "$lib/providers/authProvider.svelte";

	let { children } = $props();

    createAuthContext();
</script>

{@render children()}
