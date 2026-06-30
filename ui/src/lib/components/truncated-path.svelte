<script lang="ts">
	import { onMount } from 'svelte';

	let { path, class: className = '' }: { path: string; class?: string } = $props();

	let el: HTMLSpanElement | undefined = $state();
	let display = $state('');

	function measure(text: string, container: HTMLSpanElement): number {
		const m = document.createElement('span');
		const style = window.getComputedStyle(container);
		m.style.font = style.font;
		m.style.letterSpacing = style.letterSpacing;
		m.style.visibility = 'hidden';
		m.style.position = 'absolute';
		m.style.whiteSpace = 'nowrap';
		document.body.appendChild(m);
		m.textContent = text;
		const w = m.offsetWidth;
		document.body.removeChild(m);
		return w;
	}

	function fit(container: HTMLSpanElement) {
		const maxWidth = container.clientWidth;
		if (!maxWidth) return;

		// Try full path first
		if (measure(path, container) <= maxWidth) {
			display = path;
			return;
		}

		const parts = path.split('/').filter(Boolean);
		if (parts.length <= 2) {
			display = path;
			return;
		}

		// Progressively remove middle segments, preferring to keep more from the end
		for (let remove = 1; remove < parts.length - 1; remove++) {
			for (let startKeep = 1; startKeep <= parts.length - 1 - remove; startKeep++) {
				const endKeep = parts.length - remove - startKeep;
				if (endKeep < 1) continue;
				const candidate = '/' + parts.slice(0, startKeep).join('/') + '/\u2026/' + parts.slice(parts.length - endKeep).join('/');
				if (measure(candidate, container) <= maxWidth) {
					display = candidate;
					return;
				}
			}
		}

		display = '/' + parts[0] + '/\u2026/' + parts[parts.length - 1];
	}

	onMount(() => {
		if (!el) return;
		const ro = new ResizeObserver(() => {
			if (el) fit(el);
		});
		ro.observe(el);
		return () => ro.disconnect();
	});

	$effect(() => {
		// Re-fit when path changes
		path;
		if (el) requestAnimationFrame(() => { if (el) fit(el); });
	});
</script>

<span bind:this={el} class="block overflow-hidden whitespace-nowrap {className}" title={path}>
	{display}
</span>
