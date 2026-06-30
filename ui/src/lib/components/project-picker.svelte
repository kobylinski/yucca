<script lang="ts">
	import { fetchProjects, fetchCredentials } from '$lib/api';
	import type { ProjectInfo, CredentialMeta, ApprovalPolicy } from '$lib/api';
	import * as Drawer from '$lib/components/ui/drawer';
	import * as InputGroup from '$lib/components/ui/input-group';
	import * as Resizable from '$lib/components/ui/resizable';
	import * as ScrollArea from '$lib/components/ui/scroll-area';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import TruncatedPath from '$lib/components/truncated-path.svelte';
	import SearchIcon from '@lucide/svelte/icons/search';
	import CopyIcon from '@lucide/svelte/icons/copy';
	import ClipboardCopyIcon from '@lucide/svelte/icons/clipboard-copy';

	let {
		open = $bindable(false),
		currentProjectSlug,
		mainSelectedAlias,
		onselect,
		oncopy,
		oncreate,
		onpastevalue,
	}: {
		open: boolean;
		currentProjectSlug: string | null;
		mainSelectedAlias: string | null;
		onselect: (project: ProjectInfo) => void;
		oncopy: () => void;
		oncreate: (data: { fromProjectSlug: string; fromAlias: string; fromProjectName: string; policy: ApprovalPolicy; context: string }) => void;
		onpastevalue: (data: { fromProjectSlug: string; fromAlias: string; fromProjectName: string }) => void;
	} = $props();

	let projects: ProjectInfo[] = $state([]);
	let selectedProject: ProjectInfo | null = $state(null);
	let projectCredentials: Record<string, CredentialMeta> = $state({});
	let selectedAlias: string | null = $state(null);
	let projectSearch = $state('');
	let credentialSearch = $state('');

	let allProjects = $derived(projects.filter((p) => p.slug !== currentProjectSlug));
	let filteredProjects = $derived.by(() => {
		const q = projectSearch.toLowerCase();
		if (!q) return allProjects;
		return allProjects.filter((p) => p.name.toLowerCase().includes(q) || p.path.toLowerCase().includes(q));
	});
	let mainCredentialOpen = $derived(mainSelectedAlias !== null);
	let credentialList = $derived(Object.values(projectCredentials));
	let filteredCredentials = $derived.by(() => {
		const q = credentialSearch.toLowerCase();
		if (!q) return credentialList;
		return credentialList.filter((c) => c.alias.toLowerCase().includes(q) || (c.context ?? '').toLowerCase().includes(q));
	});
	let selectedCredential = $derived(
		selectedAlias ? projectCredentials[selectedAlias] ?? null : null
	);

	const policyLabel: Record<string, string> = {
		always_allow: 'Always allow',
		ask_session: 'Ask per session',
		ask_always: 'Ask every time',
	};

	const policyColor: Record<string, string> = {
		always_allow: '',
		ask_session: 'bg-secondary text-secondary-foreground border-secondary',
		ask_always: 'bg-primary text-primary-foreground border-primary',
	};

	async function loadProjects() {
		try {
			const result = await fetchProjects();
			projects = result ?? [];
		} catch {
			projects = [];
		}
	}

	async function selectProject(project: ProjectInfo) {
		selectedProject = project;
		selectedAlias = null;
		credentialSearch = '';
		try {
			const creds = await fetchCredentials(project.slug);
			projectCredentials = creds ?? {};
		} catch {
			projectCredentials = {};
		}
	}

	function selectCredential(alias: string) {
		selectedAlias = alias;
	}

	function handleCopyValue() {
		if (!selectedProject || !selectedCredential) return;
		onpastevalue({
			fromProjectSlug: selectedProject.slug,
			fromAlias: selectedCredential.alias,
			fromProjectName: selectedProject.name,
		});
		open = false;
	}

	function handleCopyToProject() {
		if (!selectedProject || !selectedCredential) return;
		oncreate({
			fromProjectSlug: selectedProject.slug,
			fromAlias: selectedCredential.alias,
			fromProjectName: selectedProject.name,
			policy: selectedCredential.policy,
			context: selectedCredential.context ?? '',
		});
		open = false;
	}

	$effect(() => {
		if (open) {
			selectedProject = null;
			projectCredentials = {};
			selectedAlias = null;
			projectSearch = '';
			credentialSearch = '';
			loadProjects();
		}
	});
</script>

<Drawer.Root bind:open>
	<Drawer.Content class="max-h-[80vh]">
		<div class="mx-auto w-full max-w-5xl px-6 py-4" style="height: 400px;">
			<Resizable.PaneGroup direction="horizontal" autoSaveId="yucca-picker-v2">
				<!-- Column 1: Projects -->
				<Resizable.Pane defaultSize={33} minSize={20}>
					<div class="flex h-full flex-col">
						<div class="px-1 pt-1 pb-2">
							<InputGroup.Root class="h-8 bg-transparent dark:bg-transparent border-secondary focus-within:border-secondary has-[[data-slot=input-group-control]:focus-visible]:border-secondary has-[[data-slot=input-group-control]:focus-visible]:ring-secondary/50">
								<InputGroup.Addon align="inline-start"><SearchIcon /></InputGroup.Addon>
								<InputGroup.Input type="text" placeholder="Search..." class="h-8 text-xs bg-transparent" bind:value={projectSearch} />
								<InputGroup.Addon align="inline-end">
									<span class="text-xs text-muted-foreground whitespace-nowrap">{projectSearch ? `${filteredProjects.length} matched` : `in ${allProjects.length} projects`}</span>
								</InputGroup.Addon>
							</InputGroup.Root>
						</div>
						<ScrollArea.Root class="flex-1">
							<div class="p-1 min-h-full">
								{#if filteredProjects.length === 0}
									<div class="flex min-h-[320px] items-center justify-center">
										<p class="text-sm text-muted-foreground">{projectSearch ? 'No matches' : 'No other projects'}</p>
									</div>
								{:else}
									{#each filteredProjects as project}
										<button
											type="button"
											class="flex w-full min-w-0 flex-col rounded px-3 py-2 text-left transition-colors hover:bg-accent {selectedProject?.slug === project.slug ? 'bg-accent font-medium' : ''}"
											onclick={() => selectProject(project)}
										>
											<span class="text-sm truncate w-full">{project.name}</span>
											<TruncatedPath path={project.path} class="text-xs text-muted-foreground" />
										</button>
									{/each}
								{/if}
							</div>
							<ScrollArea.Scrollbar orientation="vertical" />
						</ScrollArea.Root>
					</div>
				</Resizable.Pane>

				<Resizable.Handle withHandle />

				<!-- Column 2: Credentials -->
				<Resizable.Pane defaultSize={34} minSize={20}>
					<div class="flex h-full flex-col">
						{#if selectedProject}
							<div class="px-1 pt-1 pb-2">
								<InputGroup.Root class="h-8 bg-transparent dark:bg-transparent border-secondary focus-within:border-secondary has-[[data-slot=input-group-control]:focus-visible]:border-secondary has-[[data-slot=input-group-control]:focus-visible]:ring-secondary/50">
									<InputGroup.Addon align="inline-start"><SearchIcon /></InputGroup.Addon>
									<InputGroup.Input type="text" placeholder="Search..." class="h-8 text-xs bg-transparent" bind:value={credentialSearch} />
									<InputGroup.Addon align="inline-end">
										<span class="text-xs text-muted-foreground whitespace-nowrap">{credentialSearch ? `${filteredCredentials.length} matched` : `in ${credentialList.length} secrets`}</span>
									</InputGroup.Addon>
								</InputGroup.Root>
							</div>
						{/if}
						<ScrollArea.Root class="flex-1">
							<div class="p-1 min-h-full">
								{#if !selectedProject}
									<div class="flex min-h-[360px] items-center justify-center">
										<p class="text-sm text-muted-foreground">Select a project</p>
									</div>
								{:else if filteredCredentials.length === 0}
									<div class="flex min-h-[320px] items-center justify-center">
										<p class="text-sm text-muted-foreground">{credentialSearch ? 'No matches' : 'No secrets'}</p>
									</div>
								{:else}
									{#each filteredCredentials as cred}
										<button
											type="button"
											class="flex w-full min-w-0 flex-col rounded px-3 py-2 text-left transition-colors hover:bg-accent {selectedAlias === cred.alias ? 'bg-accent font-medium' : ''}"
											onclick={() => selectCredential(cred.alias)}
										>
											<span class="font-mono text-xs truncate w-full">{cred.alias}</span>
											{#if cred.context}
												<span class="pt-1 text-xs text-muted-foreground truncate w-full font-normal">{cred.context.split('\n')[0]}</span>
											{/if}
										</button>
									{/each}
								{/if}
							</div>
							<ScrollArea.Scrollbar orientation="vertical" />
						</ScrollArea.Root>
					</div>
				</Resizable.Pane>

				<Resizable.Handle withHandle />

				<!-- Column 3: Details -->
				<Resizable.Pane defaultSize={33} minSize={20}>
					<ScrollArea.Root class="h-full">
						<div class="p-3 min-h-full">
							{#if selectedCredential}
								<div class="space-y-2">
									<h4 class="text-sm leading-none font-medium">Secret Alias</h4>
									<p class="font-mono text-primary leading-7 break-all">{selectedCredential.alias}</p>
									<Badge variant="outline" class="truncate {policyColor[selectedCredential.policy] ?? ''}">{policyLabel[selectedCredential.policy] ?? selectedCredential.policy}</Badge>
								</div>

								{#if selectedCredential.context}
									<p class="mt-3 text-xs text-muted-foreground line-clamp-3">{selectedCredential.context.split('\n').slice(0, 3).join('\n')}</p>
								{/if}

								{#if currentProjectSlug && selectedProject && currentProjectSlug !== selectedProject.slug}
									<div class="mt-6 flex flex-col gap-2">
										{#if mainCredentialOpen}
											<Button
												size="sm"
												variant="secondary"
												onclick={handleCopyValue}
												>
												<ClipboardCopyIcon /> Copy Secret Value
											</Button>
										{/if}
											<Button
											size="sm"
											variant="secondary"
											onclick={handleCopyToProject}
											>
											<CopyIcon /> Copy to Project
										</Button>
									</div>
								{/if}
							{:else}
								<div class="flex min-h-[360px] items-center justify-center">
									<p class="text-sm text-muted-foreground">Select a secret</p>
								</div>
							{/if}
						</div>
						<ScrollArea.Scrollbar orientation="vertical" />
					</ScrollArea.Root>
				</Resizable.Pane>
			</Resizable.PaneGroup>
		</div>

	</Drawer.Content>
</Drawer.Root>
