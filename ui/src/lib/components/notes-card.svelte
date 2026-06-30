<script lang="ts">
	import { fetchNotes, setNote, deleteNote, type Note } from '$lib/api';
	import * as Card from '$lib/components/ui/card';
	import { Input } from '$lib/components/ui/input';
	import { Textarea } from '$lib/components/ui/textarea';
	import { Button } from '$lib/components/ui/button';
	import { Separator } from '$lib/components/ui/separator';
	import StickyNoteIcon from '@lucide/svelte/icons/sticky-note';
	import Trash2Icon from '@lucide/svelte/icons/trash-2';
	import SaveIcon from '@lucide/svelte/icons/save';
	import PlusIcon from '@lucide/svelte/icons/plus';
	import PencilLineIcon from '@lucide/svelte/icons/pencil-line';
	import XIcon from '@lucide/svelte/icons/x';

	let { slug, refresh = 0 }: { slug: string | null; refresh?: number } = $props();

	let notes: Note[] = $state([]);
	let newAlias = $state('');
	let newBody = $state('');
	let editingAlias: string | null = $state(null);
	let editBody = $state('');

	$effect(() => {
		const s = slug;
		void refresh; // also re-fetch when the parent signals a notes_changed event
		if (s) load(s);
	});

	async function load(s: string) {
		try {
			notes = (await fetchNotes(s)) ?? [];
		} catch {
			notes = [];
		}
	}

	async function add() {
		const alias = newAlias.trim();
		if (!slug || !alias) return;
		await setNote(slug, alias, newBody);
		newAlias = '';
		newBody = '';
		await load(slug);
	}

	function startEdit(n: Note) {
		editingAlias = n.alias;
		editBody = n.body;
	}

	async function saveEdit(alias: string) {
		if (!slug) return;
		await setNote(slug, alias, editBody);
		editingAlias = null;
		await load(slug);
	}

	async function remove(alias: string) {
		if (!slug) return;
		await deleteNote(slug, alias);
		await load(slug);
	}
</script>

<Card.Root class="mt-6 w-full max-w-4xl p-6">
	<div class="mb-4 flex items-center gap-2">
		<StickyNoteIcon class="text-primary size-4" />
		<h3 class="text-sm font-medium">Notes</h3>
		<span class="text-muted-foreground text-xs">non-secret · project-scoped</span>
	</div>

	<div class="mb-4 flex flex-col gap-2 sm:flex-row sm:items-start">
		<Input bind:value={newAlias} placeholder="key (e.g. staging-db)" class="sm:max-w-[200px]" />
		<Textarea bind:value={newBody} placeholder="note text" rows={1} class="min-h-9 flex-1" />
		<Button size="sm" onclick={add} disabled={!newAlias.trim()}>
			<PlusIcon class="size-4" /> Add
		</Button>
	</div>

	<Separator class="mb-4" />

	{#if notes.length === 0}
		<p class="text-muted-foreground text-sm">
			No notes yet. Add one above — or the agent can save one with the trustee_note_store tool.
		</p>
	{:else}
		<ul class="space-y-2">
			{#each notes as note (note.alias)}
				<li class="rounded-md border p-3">
					<div class="flex items-start justify-between gap-2">
						<div class="min-w-0 flex-1">
							<div class="text-muted-foreground font-mono text-xs">{note.alias}</div>
							{#if editingAlias === note.alias}
								<Textarea bind:value={editBody} rows={2} class="mt-1" />
							{:else}
								<div class="mt-1 text-sm whitespace-pre-wrap">{note.body}</div>
							{/if}
						</div>
						<div class="flex shrink-0 gap-1">
							{#if editingAlias === note.alias}
								<Button size="icon" variant="ghost" onclick={() => saveEdit(note.alias)}>
									<SaveIcon class="size-4" />
								</Button>
								<Button size="icon" variant="ghost" onclick={() => (editingAlias = null)}>
									<XIcon class="size-4" />
								</Button>
							{:else}
								<Button size="icon" variant="ghost" onclick={() => startEdit(note)}>
									<PencilLineIcon class="size-4" />
								</Button>
								<Button size="icon" variant="ghost" onclick={() => remove(note.alias)}>
									<Trash2Icon class="size-4" />
								</Button>
							{/if}
						</div>
					</div>
				</li>
			{/each}
		</ul>
	{/if}
</Card.Root>
