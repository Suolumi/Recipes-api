<script lang="ts">
	const { children } = $props();
</script>

<div class="flex h-screen w-full items-center justify-center">
	{@render children()}
</div>
