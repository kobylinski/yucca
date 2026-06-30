<script lang="ts">
	import { page } from '$app/stores';
	import { goto } from '$app/navigation';
	import { onMount } from 'svelte';

	const id = $derived($page.params.id);

	onMount(() => {
		goto(`/?request=${id}`, { replaceState: true });
	});
</script>

<div class="flex min-h-screen items-center justify-center">
	<p class="text-muted-foreground">Redirecting...</p>
</div>
