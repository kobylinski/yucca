<script lang="ts">
	import { onMount } from 'svelte';
	import { fade } from 'svelte/transition';
	import { cubicInOut } from 'svelte/easing';
	import {
		fetchCredentials,
		fetchPendingRequests,
		fetchSessions,
		approveRequest,
		denyRequest,
		updateCredential,
		deleteCredential,
		revealCredential,
		updateCredentialContext,
		createCredential,
	} from '$lib/api';
	import type { ApprovalPolicy, SecretRequest, CredentialMeta, ActiveSession } from '$lib/api';

	function contextPreview(text: string | undefined, maxLines = 3): string {
		if (!text) return '';
		const lines = text.split('\n').slice(0, maxLines);
		return lines.join('\n');
	}
	import * as Alert from '$lib/components/ui/alert';
	import * as Card from '$lib/components/ui/card';
	import * as RadioGroup from '$lib/components/ui/radio-group';
	import * as InputGroup from '$lib/components/ui/input-group';
	import * as Item from '$lib/components/ui/item';
	import * as Tabs from '$lib/components/ui/tabs';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import { Label } from '$lib/components/ui/label';
	import { Separator } from '$lib/components/ui/separator';
	import { Textarea } from '$lib/components/ui/textarea';
	import ProjectPicker from '$lib/components/project-picker.svelte';
	import NotesCard from '$lib/components/notes-card.svelte';
	import KeyRoundIcon from '@lucide/svelte/icons/key-round';
	import EyeIcon from '@lucide/svelte/icons/eye';
	import EyeOffIcon from '@lucide/svelte/icons/eye-off';
	import RefreshCwIcon from '@lucide/svelte/icons/refresh-cw';
	import SaveIcon from '@lucide/svelte/icons/save';
	import Trash2Icon from '@lucide/svelte/icons/trash-2';
	import XIcon from '@lucide/svelte/icons/x';
	import PlusIcon from '@lucide/svelte/icons/plus';
	import PencilLineIcon from '@lucide/svelte/icons/pencil-line';
	import AlertCircleIcon from '@lucide/svelte/icons/alert-circle';
	import TerminalIcon from '@lucide/svelte/icons/terminal';
	import ShieldAlertIcon from '@lucide/svelte/icons/shield-alert';

	import CoffeeIcon from '@lucide/svelte/icons/coffee';
	import { toast } from 'svelte-sonner';
	import BotIcon from '@lucide/svelte/icons/bot';

	let sessions: ActiveSession[] = $state([]);
	let currentProjectSlug = $state<string | null>(null);
	// Credentials keyed by project slug, then by alias
	let allCredentials: Record<string, Record<string, CredentialMeta>> = $state({});
	let selectedAlias: string | null = $state(null);
	let secretValue = $state('');
	let selectedPolicy: ApprovalPolicy = $state('ask_session');
	let pendingRequests: SecretRequest[] = $state([]);
	let interruptMode = $state(false);
	let currentRequestIndex = $state(0);
	let interruptSecretValue = $state('');
	let interruptPolicy: ApprovalPolicy = $state('ask_session');
	let loading = $state(true);
	let drawerOpen = $state(false);
	let originalTitle = '';
	let revealedValue: string | null = $state(null);
	let revealing = $state(false);
	let editingContext = $state(false);
	let contextDraft = $state('');
	let contextHint = $state(false);
	let contextHintTimer: ReturnType<typeof setTimeout> | null = null;
	let creatingCredential = $state(false);
	let createAlias = $state('');
	let createValue = $state('');
	let createPolicy: ApprovalPolicy = $state('ask_session');
	let createContext = $state('');
	let copyFrom: { projectSlug: string; alias: string; projectName: string } | null = $state(null);
	let pasteValueFrom: { projectSlug: string; alias: string; projectName: string } | null = $state(null);
	let notesRefresh = $state(0);

	function flashContextHint() {
		if (contextHintTimer) clearTimeout(contextHintTimer);
		contextHint = true;
		contextHintTimer = setTimeout(() => { contextHint = false; }, 2000);
	}

	let credentials = $derived(allCredentials[currentProjectSlug ?? ''] ?? {});
	let credentialList = $derived(Object.values(credentials));
	let aliasExists = $derived(createAlias !== '' && credentials[createAlias] !== undefined);
	const aliasPattern = /^[A-Za-z0-9_\-.]+$/;
	let aliasInvalid = $derived(createAlias !== '' && !aliasPattern.test(createAlias));
	let aliasTooLong = $derived(createAlias.length > 64);
	let selectedCredential = $derived(
		selectedAlias ? credentials[selectedAlias] ?? null : null
	);
	let isFileSource = $derived(selectedCredential?.source?.type === 'file');
	let currentRequest = $derived(
		pendingRequests.length > currentRequestIndex ? pendingRequests[currentRequestIndex] : null
	);
	let remainingRequests = $derived(
		Math.max(0, pendingRequests.length - currentRequestIndex)
	);
	let isExecRequest = $derived(currentRequest?.kind === 'execute_accept');
	let execCommand = $derived(isExecRequest && currentRequest?.reason?.startsWith('exec: ') ? currentRequest!.reason.slice(6) : currentRequest?.reason ?? '');

	// Sidebar entries — each request is now its own entry (no grouping needed)
	interface PendingGroup {
		type: 'exec' | 'secret';
		label: string;
		index: number;
		count: number;
	}
	let pendingGroups = $derived.by((): PendingGroup[] => {
		return pendingRequests.map((req, i) => {
			if (req.kind === 'execute_accept') {
				return { type: 'exec' as const, label: req.reason?.startsWith('exec: ') ? req.reason.slice(6) : req.reason, index: i, count: req.aliases?.length ?? 0 };
			}
			return { type: 'secret' as const, label: req.alias ?? '', index: i, count: 1 };
		});
	});

	const faviconDefault = `data:image/svg+xml,${encodeURIComponent('<svg xmlns="http://www.w3.org/2000/svg" width="32" height="32" viewBox="0 0 96 96" fill="none" stroke="#e11d48" stroke-width="8" stroke-linecap="round"><line x1="48" y1="48" x2="80" y2="48"/><line x1="48" y1="48" x2="70.63" y2="70.63"/><line x1="48" y1="48" x2="48" y2="80"/><line x1="48" y1="48" x2="25.37" y2="70.63"/><line x1="48" y1="48" x2="16" y2="48"/><line x1="48" y1="48" x2="25.37" y2="25.37"/><line x1="48" y1="48" x2="48" y2="16"/><line x1="48" y1="48" x2="70.63" y2="25.37"/><line x1="48" y1="48" x2="72.02" y2="57.95"/><line x1="48" y1="48" x2="57.95" y2="72.02"/><line x1="48" y1="48" x2="38.05" y2="72.02"/><line x1="48" y1="48" x2="23.98" y2="57.95"/><line x1="48" y1="48" x2="23.98" y2="38.05"/><line x1="48" y1="48" x2="38.05" y2="23.98"/><line x1="48" y1="48" x2="57.95" y2="23.98"/><line x1="48" y1="48" x2="72.02" y2="38.05"/></svg>')}`;
	const faviconPending = `data:image/svg+xml,${encodeURIComponent('<svg xmlns="http://www.w3.org/2000/svg" width="32" height="32" viewBox="0 0 96 96" fill="none" stroke="#f97316" stroke-width="8" stroke-linecap="round"><line x1="48" y1="48" x2="80" y2="48"/><line x1="48" y1="48" x2="70.63" y2="70.63"/><line x1="48" y1="48" x2="48" y2="80"/><line x1="48" y1="48" x2="25.37" y2="70.63"/><line x1="48" y1="48" x2="16" y2="48"/><line x1="48" y1="48" x2="25.37" y2="25.37"/><line x1="48" y1="48" x2="48" y2="16"/><line x1="48" y1="48" x2="70.63" y2="25.37"/><line x1="48" y1="48" x2="72.02" y2="57.95"/><line x1="48" y1="48" x2="57.95" y2="72.02"/><line x1="48" y1="48" x2="38.05" y2="72.02"/><line x1="48" y1="48" x2="23.98" y2="57.95"/><line x1="48" y1="48" x2="23.98" y2="38.05"/><line x1="48" y1="48" x2="38.05" y2="23.98"/><line x1="48" y1="48" x2="57.95" y2="23.98"/><line x1="48" y1="48" x2="72.02" y2="38.05"/></svg>')}`;
	let favicon = $derived(pendingRequests.length > 0 ? faviconPending : faviconDefault);

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

	function maskedValue(length: number): string {
		if (length === 0) return '';
		return '\u2022'.repeat(Math.min(length, 24));
	}

	let ws: WebSocket | null = null;

	onMount(() => {
		originalTitle = document.title;
		initLoad();
		return () => ws?.close();
	});

	function connectWebSocket() {
		const wsURL = `ws://${window.location.host}/api/ws`;
		ws = new WebSocket(wsURL);

		ws.onmessage = (event) => {
			const msg = JSON.parse(event.data);
			switch (msg.type) {
				case 'request_created':
				case 'request_resolved':
					loadPendingRequests();
					break;
				case 'credentials_changed':
					loadCredentialsForProject(msg.project);
					break;
				case 'notes_changed':
					notesRefresh++;
					break;
				case 'sessions_changed':
					loadSessions();
					break;
			}
		};

		ws.onclose = () => {
			setTimeout(connectWebSocket, 2000);
		};
	}

	let pendingFetchTimer: ReturnType<typeof setTimeout> | null = null;

	function loadPendingRequests() {
		// Debounce: batch rapid WS events into one fetch
		if (pendingFetchTimer) clearTimeout(pendingFetchTimer);
		pendingFetchTimer = setTimeout(async () => {
			const pending = await fetchPendingRequests();
			const wasEmpty = pendingRequests.length === 0;
			syncPending(pending);
			if (pending.length > 0 && wasEmpty) {
				interruptMode = true;
				currentRequestIndex = 0;
				notifyTab();
			}
		}, 200);
	}

	let titleFlashInterval: ReturnType<typeof setInterval> | null = null;

	function notifyTab() {
		if (document.hasFocus()) return;

		let flash = true;
		if (titleFlashInterval) clearInterval(titleFlashInterval);
		titleFlashInterval = setInterval(() => {
			document.title = flash ? '🔑 Secret Requested' : originalTitle;
			flash = !flash;
		}, 800);

		const onFocus = () => {
			restoreTitle();
			window.removeEventListener('focus', onFocus);
		};
		window.addEventListener('focus', onFocus);
	}

	function restoreTitle() {
		if (titleFlashInterval) {
			clearInterval(titleFlashInterval);
			titleFlashInterval = null;
		}
		document.title = originalTitle;
	}

	async function initLoad() {
		try {
			// Load sessions first, then credentials for all sessions
			await loadSessions();
			await loadAllCredentials();
			await pollRequests();
		} catch {
			// daemon may not be running
		} finally {
			loading = false;
			connectWebSocket();
		}
	}

	async function loadSessions() {
		try {
			const result = await fetchSessions();
			sessions = result ?? [];
			if (sessions.length === 0) {
				// All sessions ended — reset UI
				currentProjectSlug = null;
				resetSelection();
			} else if (!currentProjectSlug) {
				currentProjectSlug = sessions[0].project_slug;
			} else if (!sessions.find(s => s.project_slug === currentProjectSlug)) {
				// Current project's session ended — switch to first available
				currentProjectSlug = sessions[0].project_slug;
				resetSelection();
			}
			// Load credentials for any new sessions
			for (const s of sessions) {
				if (!(s.project_slug in allCredentials)) {
					loadCredentialsForProject(s.project_slug);
				}
			}
		} catch {
			sessions = [];
		}
	}

	function resetSelection() {
		selectedAlias = null;
		secretValue = '';
		revealedValue = null;
		editingContext = false;
		creatingCredential = false;
		pasteValueFrom = null;
	}

	async function loadAllCredentials() {
		await Promise.all(sessions.map((s) => loadCredentialsForProject(s.project_slug)));
	}

	async function loadCredentialsForProject(slug: string) {
		if (!slug) return;
		try {
			const creds = await fetchCredentials(slug);
			allCredentials = { ...allCredentials, [slug]: creds ?? {} };
		} catch {
			allCredentials = { ...allCredentials, [slug]: {} };
		}
	}

	async function pollRequests() {
		try {
			await loadPendingRequests();
		} catch {
			// ignore
		}
	}

	function syncPending(pending: import('$lib/api').SecretRequest[]) {
		pendingRequests = pending;
		if (pending.length === 0) {
			interruptMode = false;
			currentRequestIndex = 0;
			restoreTitle();
		} else if (currentRequestIndex >= pending.length) {
			currentRequestIndex = 0;
		}
		interruptSecretValue = '';
		interruptPolicy = 'ask_session';
	}

	async function handleApprove() {
		if (!currentRequest) return;
		if (isExecRequest) {
			const resp = await approveRequest(currentRequest.id, '', 'ask_session');
			syncPending(resp.pending);
			toast.success('Execution approved');
		} else {
			const alias = currentRequest.alias;
			const resp = await approveRequest(currentRequest.id, interruptSecretValue, interruptPolicy);
			syncPending(resp.pending);
			toast.success(`Secret "${alias}" approved`);
		}
	}

	async function handleDeny() {
		if (!currentRequest) return;
		if (isExecRequest) {
			const resp = await denyRequest(currentRequest.id);
			syncPending(resp.pending);
			toast.error('Execution denied');
		} else {
			const alias = currentRequest.alias;
			const resp = await denyRequest(currentRequest.id);
			syncPending(resp.pending);
			toast.error(`Secret "${alias}" denied`);
		}
	}

	function selectProject(slug: string) {
		currentProjectSlug = slug;
		selectedAlias = null;
		revealedValue = null;
	}

	function selectCredential(alias: string) {
		selectedAlias = alias;
		revealedValue = null;
		editingContext = false;
		const cred = credentials[alias];
		if (cred) {
			selectedPolicy = cred.policy;
			secretValue = '';
		}
	}

	async function handleReveal() {
		if (!currentProjectSlug || !selectedAlias) return;
		if (revealedValue !== null) {
			revealedValue = null;
			return;
		}
		revealing = true;
		try {
			revealedValue = await revealCredential(currentProjectSlug, selectedAlias);
		} catch {
			revealedValue = null;
		} finally {
			revealing = false;
		}
	}

	async function handleSave() {
		if (!currentProjectSlug || !selectedAlias) return;
		const payload: { value?: string; policy?: ApprovalPolicy; copy_value_from?: { project_slug: string; alias: string } } = {
			policy: selectedPolicy,
		};
		if (pasteValueFrom) {
			payload.copy_value_from = { project_slug: pasteValueFrom.projectSlug, alias: pasteValueFrom.alias };
		} else if (secretValue) {
			payload.value = secretValue;
		}
		await updateCredential(currentProjectSlug, selectedAlias, payload);
		secretValue = '';
		pasteValueFrom = null;
		// Reload credentials locally since we used No-Emit
		await loadCredentialsForProject(currentProjectSlug);
	}

	async function handleRefresh() {
		if (!currentProjectSlug || !selectedAlias) return;
		// Re-read file value — don't suppress WS event so other UIs update too
		await updateCredential(currentProjectSlug, selectedAlias, { policy: selectedPolicy }, false);
		revealedValue = null;
	}

	function openContextEditor(cred: CredentialMeta) {
		contextDraft = cred.context ?? '';
		editingContext = true;
	}

	function closeContextEditor() {
		editingContext = false;
	}

	async function handleSaveContext() {
		if (!currentProjectSlug || !selectedAlias) return;
		await updateCredentialContext(currentProjectSlug, selectedAlias, contextDraft);
		await loadCredentialsForProject(currentProjectSlug);
		editingContext = false;
	}

	function enterCreateMode(prefill?: { alias?: string; policy?: ApprovalPolicy; context?: string; copyFrom?: { projectSlug: string; alias: string; projectName: string } }) {
		selectedAlias = null;
		revealedValue = null;
		editingContext = false;
		creatingCredential = true;
		createAlias = prefill?.alias ?? '';
		createValue = '';
		createPolicy = prefill?.policy ?? 'ask_session';
		createContext = prefill?.context ?? '';
		copyFrom = prefill?.copyFrom ?? null;
		pasteValueFrom = null;
	}

	function cancelCreate() {
		creatingCredential = false;
		createAlias = '';
		createValue = '';
		createContext = '';
		copyFrom = null;
	}

	async function handleCreate() {
		if (!currentProjectSlug || !createAlias || aliasExists) return;

		try {
			await createCredential(currentProjectSlug, {
				alias: createAlias,
				...(copyFrom
					? { copy_from: { project_slug: copyFrom.projectSlug, alias: copyFrom.alias } }
					: { value: createValue }),
				policy: createPolicy,
				context: createContext || undefined,
			});
			creatingCredential = false;
			copyFrom = null;
			createAlias = '';
			createValue = '';
			createContext = '';
			await loadCredentialsForProject(currentProjectSlug);
		} catch {
			// duplicate or other error — aliasExists should already show UI feedback
		}
	}

	function applyPasteValue(source: { fromProjectSlug: string; fromAlias: string; fromProjectName: string }) {
		pasteValueFrom = { projectSlug: source.fromProjectSlug, alias: source.fromAlias, projectName: source.fromProjectName };
	}

	async function handleDelete() {
		if (!currentProjectSlug || !selectedAlias) return;
		await deleteCredential(currentProjectSlug, selectedAlias);
		selectedAlias = null;
		secretValue = '';
		revealedValue = null;
		// Reload credentials locally since we used No-Emit
		await loadCredentialsForProject(currentProjectSlug);
	}
</script>

<svelte:head>
	<link rel="icon" href={favicon} />
</svelte:head>

<div class="flex min-h-screen flex-col items-center justify-center p-6">
	{#if loading}
		<p class="text-muted-foreground text-sm">Loading...</p>
	{:else}
		<!-- Project tabs -->
		{#if sessions.length > 0}
			<Tabs.Root value={currentProjectSlug ?? ''} onValueChange={(v) => selectProject(v)}>
				<Tabs.List class="mb-4">
					{#each sessions as session}
						<Tabs.Trigger value={session.project_slug}>
							{session.project_name}
						</Tabs.Trigger>
					{/each}
				</Tabs.List>
			</Tabs.Root>
		{/if}

		{#if sessions.length === 0}
			<Card.Root class="w-full max-w-md p-8">
				<div class="flex flex-col items-center gap-4 text-center">
					<CoffeeIcon class="size-10 text-muted-foreground" />
					<div>
						<h3 class="text-lg font-medium">No active sessions</h3>
						<p class="text-sm text-muted-foreground mt-1">Start an MCP agent session to manage secrets here.</p>
					</div>
				</div>
			</Card.Root>
		{:else}
		<Card.Root class="w-full max-w-4xl overflow-hidden p-0 min-h-[50vh]">
			<div class="grid min-h-[50vh] md:grid-cols-2">
				<!-- Left panel -->
				<div class="border-r p-6 flex flex-col">
					{#key interruptMode}
						<div class="flex-1 flex flex-col" in:fade={{ duration: 300, easing: cubicInOut }} out:fade={{ duration: 200, easing: cubicInOut }}>
							{#if interruptMode && currentRequest}
								{#if isExecRequest}
									<!-- Exec approval: show command + all credentials -->
									<div class="flex items-center gap-2 mb-4">
										<TerminalIcon class="size-5 text-primary" />
										<div>
											<small class="text-sm leading-none font-medium">Command Execution</small>
										</div>
									</div>

									<Separator class="mb-4" />

									<div class="flex flex-col gap-4">
										<div>
											<small class="text-sm leading-none font-medium">Command</small>
											<pre class="mt-1 rounded-md bg-muted p-3 text-sm font-mono whitespace-pre-wrap break-all">{execCommand}</pre>
										</div>

										<Separator />

										<div>
											<small class="text-sm leading-none font-medium">Credentials ({currentRequest.aliases?.length ?? 0})</small>
											<div class="mt-2 flex flex-col gap-1">
												{#each currentRequest.aliases ?? [] as alias}
													<div class="flex items-center gap-2 rounded-md bg-muted/50 px-3 py-2">
														<ShieldAlertIcon class="size-4 text-muted-foreground shrink-0" />
														<code class="text-sm font-mono">{alias}</code>
													</div>
												{/each}
											</div>
										</div>

										<div class="flex gap-2 pt-2">
											<Button variant="secondary" onclick={handleApprove}>
												Approve
											</Button>
											<Button variant="destructive" onclick={handleDeny}>
												Deny
											</Button>
										</div>
									</div>
								{:else}
									<!-- Regular secret request -->
									<div class="flex items-center gap-2 mb-4">
										<KeyRoundIcon class="size-5 text-primary" />
										<div>
											<small class="text-sm leading-none font-medium">Secret Request</small>
											<p class="font-mono leading-7">{currentRequest?.alias}</p>
										</div>
									</div>

									<Separator class="mb-4" />

									<div class="space-y-4">
										<div>
											<small class="text-sm leading-none font-medium">Reason</small>
											<p class="text-muted-foreground leading-7">"{currentRequest?.reason}"</p>
										</div>

										<Separator />

										<div class="space-y-2">
											<Label for="interrupt-secret">Secret</Label>
											<InputGroup.Root>
												<InputGroup.Input
													id="interrupt-secret"
													type="password"
													placeholder="Enter secret value"
													bind:value={interruptSecretValue}
												/>
											</InputGroup.Root>
										</div>

										<div class="space-y-2">
											<Label>Policy</Label>
											<RadioGroup.Root bind:value={interruptPolicy}>
												<div class="flex items-center space-x-2">
													<RadioGroup.Item value="always_allow" id="int-always" />
													<Label for="int-always">Always allow</Label>
												</div>
												<div class="flex items-center space-x-2">
													<RadioGroup.Item value="ask_session" id="int-session" />
													<Label for="int-session">Ask per session</Label>
												</div>
												<div class="flex items-center space-x-2">
													<RadioGroup.Item value="ask_always" id="int-always-ask" />
													<Label for="int-always-ask">Ask every time</Label>
												</div>
											</RadioGroup.Root>
										</div>

										<div class="flex gap-2 pt-2">
											<Button variant="secondary" onclick={handleApprove}>
												Approve
											</Button>
											<Button variant="destructive" onclick={handleDeny}>
												Deny
											</Button>
										</div>
									</div>
								{/if}
							{:else if creatingCredential}
								<div class="mb-4">
									<div class="space-y-2">
										<h4 class="text-sm leading-none font-medium">New Secret</h4>
										<InputGroup.Root>
											<InputGroup.Input
												type="text"
												placeholder="Alias name"
												class="font-mono"
												bind:value={createAlias}
											/>
										</InputGroup.Root>
										{#if aliasExists}
											<p class="text-xs text-destructive">A credential with this alias already exists</p>
										{:else if aliasInvalid}
											<p class="text-xs text-destructive">Only letters, numbers, _ - . allowed</p>
										{:else if aliasTooLong}
											<p class="text-xs text-destructive">Maximum 64 characters</p>
										{/if}
									</div>
								</div>

								<Separator class="mb-4" />

								<div class="space-y-4">
									{#if copyFrom}
										<div class="space-y-2">
											<Label>Value Source</Label>
											<p class="text-sm text-muted-foreground">
												Value from <span class="font-medium text-foreground">{copyFrom.projectName}</span> / <code class="text-xs">{copyFrom.alias}</code>
											</p>
										</div>
									{:else}
										<div class="space-y-2">
											<Label for="create-val">Secret Value</Label>
											<InputGroup.Root>
												<InputGroup.Input
													id="create-val"
													type="password"
													placeholder="Enter secret value"
													bind:value={createValue}
												/>
											</InputGroup.Root>
										</div>
									{/if}

									<div class="space-y-2">
										<h4 class="text-sm leading-none font-medium">Approval Policy</h4>
										<RadioGroup.Root bind:value={createPolicy} class="pt-2">
											<div class="flex items-center space-x-2">
												<RadioGroup.Item value="always_allow" id="create-always" />
												<Label for="create-always">Always allow</Label>
											</div>
											<div class="flex items-center space-x-2">
												<RadioGroup.Item value="ask_session" id="create-session" />
												<Label for="create-session">Ask per session</Label>
											</div>
											<div class="flex items-center space-x-2">
												<RadioGroup.Item value="ask_always" id="create-always-ask" />
												<Label for="create-always-ask">Ask every time</Label>
											</div>
										</RadioGroup.Root>
									</div>

									<div class="flex justify-center gap-2 pt-2">
										<Button onclick={handleCreate} disabled={!createAlias || aliasExists || aliasInvalid || aliasTooLong}>
											<SaveIcon /> Create
										</Button>
										<Button variant="secondary" onclick={cancelCreate}>
											Cancel
										</Button>
									</div>
								</div>
							{:else if selectedCredential && selectedAlias}
								<div class="mb-4">
									<div class="space-y-2">
										<h4 class="text-sm leading-none font-medium">Secret Alias</h4>
										<p class="font-mono text-primary leading-7 break-all">{selectedCredential.alias}</p>
									</div>
									{#if editingContext}
										<span class="mt-2 inline-flex items-center gap-1.5 text-xs text-muted-foreground">
											<PencilLineIcon class="size-3" /> editing context...
										</span>
									{:else if selectedCredential.context}
										<button
											type="button"
											class="mt-2 w-full text-left font-normal text-xs text-muted-foreground line-clamp-3 whitespace-pre-wrap cursor-pointer hover:text-foreground transition-colors"
											onclick={() => openContextEditor(selectedCredential)}
										>{contextPreview(selectedCredential.context)}</button>
									{:else}
										<button
											type="button"
											class="mt-2 inline-flex items-center gap-1 text-xs text-muted-foreground/50 hover:text-muted-foreground transition-colors"
											onclick={() => openContextEditor(selectedCredential)}
										><PlusIcon class="size-3" /> add context</button>
									{/if}
								</div>

								<Separator class="mb-4" />

								<!-- svelte-ignore a11y_no_static_element_interactions -->
								<div class="relative" onclick={editingContext ? flashContextHint : undefined}>
									{#if editingContext}
										<div class="absolute inset-0 z-10 cursor-not-allowed"></div>
									{/if}
									<div class="space-y-4 {editingContext ? 'opacity-40 select-none' : ''}">
										<div class="space-y-2">
											<h4 class="text-sm leading-none font-medium">Approval Policy</h4>
											<small class="text-muted-foreground font-normal">When should Yucca prompt for approval?</small>
											<RadioGroup.Root bind:value={selectedPolicy} class="pt-4">
												<div class="flex items-center space-x-2">
													<RadioGroup.Item value="always_allow" id="pol-always" />
													<Label for="pol-always">Always allow</Label>
												</div>
												<div class="flex items-center space-x-2">
													<RadioGroup.Item value="ask_session" id="pol-session" />
													<Label for="pol-session">Ask per session</Label>
												</div>
												<div class="flex items-center space-x-2">
													<RadioGroup.Item value="ask_always" id="pol-always-ask" />
													<Label for="pol-always-ask">Ask every time</Label>
												</div>
											</RadioGroup.Root>
										</div>

										<Separator />

										<div class="space-y-2">
											<Label>Current Value</Label>
											<InputGroup.Root>
												<InputGroup.Input
													type={revealedValue !== null ? 'text' : 'password'}
													value={revealedValue ?? maskedValue(selectedCredential.value_length)}
													readonly
												/>
												<InputGroup.Addon align="inline-end">
													<InputGroup.Button
														size="icon-xs"
														variant="ghost"
														onclick={handleReveal}
														disabled={revealing}
													>
														{#if revealedValue !== null}
															<EyeOffIcon />
														{:else}
															<EyeIcon />
														{/if}
													</InputGroup.Button>
												</InputGroup.Addon>
											</InputGroup.Root>
										</div>

										{#if !isFileSource}
											<div class="space-y-2">
												<Label for="secret-val">New Value</Label>
												{#if pasteValueFrom}
													<p class="text-sm text-muted-foreground">
														Value from <span class="font-medium text-foreground">{pasteValueFrom.projectName}</span> / <code class="text-xs">{pasteValueFrom.alias}</code>
													</p>
												{:else}
													<InputGroup.Root>
														<InputGroup.Input
															id="secret-val"
															type="password"
															placeholder="Enter new value to update"
															bind:value={secretValue}
														/>
													</InputGroup.Root>
												{/if}
											</div>
										{/if}

										<div class="flex justify-center gap-2 pt-2">
											<Button onclick={handleSave}>
												<SaveIcon /> Save
											</Button>
											{#if isFileSource}
												<Button variant="secondary" onclick={handleRefresh}>
													<RefreshCwIcon /> Refresh
												</Button>
											{:else}
												<Button variant="secondary" onclick={handleDelete}>
													<Trash2Icon /> Delete
												</Button>
											{/if}
											<Button variant="secondary" onclick={() => { selectedAlias = null; revealedValue = null; editingContext = false; pasteValueFrom = null; }}>
												Cancel
											</Button>
										</div>
									</div>
								</div>
							{:else}
								<div class="flex flex-1 flex-col items-center justify-center gap-3">
									<p class="text-muted-foreground text-sm">Select a credential from the list</p>
									<Button variant="outline" size="sm" onclick={() => enterCreateMode()}>
										<PlusIcon /> Create new secret
									</Button>
								</div>
							{/if}
						</div>
					{/key}
				</div>

				<!-- Right panel: credential list or context editor -->
				<div class="flex flex-col p-6">
					{#if interruptMode}
						<div class="flex flex-col gap-1">
							<small class="text-xs text-muted-foreground font-medium mb-2">Pending ({pendingGroups.length})</small>
							{#each pendingGroups as group}
								<button
									type="button"
									class="flex items-center gap-2 rounded-md px-3 py-2 text-left text-sm transition-colors {currentRequestIndex === group.index ? 'bg-accent' : 'hover:bg-muted'}"
									onclick={() => { currentRequestIndex = group.index; interruptSecretValue = ''; interruptPolicy = 'ask_session'; }}
								>
									{#if group.type === 'exec'}
										<TerminalIcon class="size-4 shrink-0 text-muted-foreground" />
										<span class="font-mono truncate">{group.label}</span>
										{#if group.count > 1}
											<Badge variant="outline" class="ml-auto shrink-0">{group.count}</Badge>
										{/if}
									{:else}
										<KeyRoundIcon class="size-4 shrink-0 text-muted-foreground" />
										<span class="font-mono truncate">{group.label}</span>
									{/if}
								</button>
							{/each}
						</div>
					{:else if (editingContext && selectedCredential) || creatingCredential}
						<div class="flex flex-col flex-1">
							<div class="flex justify-end gap-1.5 mb-3">
								{#if creatingCredential}
									<span class="text-xs text-muted-foreground self-center mr-auto">Context (optional)</span>
								{:else}
									<Button variant="ghost" size="icon" class="size-8" onclick={closeContextEditor}>
										<XIcon />
									</Button>
									<Button variant="ghost" size="icon" class="size-8 text-primary" onclick={handleSaveContext}>
										<SaveIcon />
									</Button>
								{/if}
							</div>
							{#if contextHint && !creatingCredential}
								<div transition:fade={{ duration: 150 }}>
									<Alert.Root class="mb-3">
										<AlertCircleIcon class="text-primary" />
										<Alert.Title>Close or save context first</Alert.Title>
									</Alert.Root>
								</div>
							{/if}
							<div class="flex-1 rounded-md border bg-muted/30 p-1">
								{#if creatingCredential}
									<textarea
										class="size-full min-h-[250px] resize-none bg-transparent p-3 text-sm font-mono leading-relaxed text-foreground placeholder:text-muted-foreground/50 focus:outline-none"
										bind:value={createContext}
										placeholder="Describe what this credential is used for, where it comes from, any notes for future reference..."
									></textarea>
								{:else}
									<textarea
										class="size-full min-h-[250px] resize-none bg-transparent p-3 text-sm font-mono leading-relaxed text-foreground placeholder:text-muted-foreground/50 focus:outline-none"
										bind:value={contextDraft}
										placeholder="Describe what this credential is used for, where it comes from, any notes for future reference..."
									></textarea>
								{/if}
							</div>
						</div>
					{:else if credentialList.length === 0}
						<div class="flex flex-1 items-center justify-center">
							<p class="text-muted-foreground text-sm">No credentials stored</p>
						</div>
					{:else}
						<Item.Group>
							{#each credentialList as cred, index}
								<Item.Root class="w-full min-w-0 cursor-pointer {selectedAlias === cred.alias ? 'bg-accent' : ''}">
									{#snippet child({ props }: { props: Record<string, unknown> })}
										<button type="button" {...props} onclick={() => selectedAlias === cred.alias ? (selectedAlias = null, revealedValue = null, editingContext = false) : selectCredential(cred.alias)}>
											<Item.Content class="min-w-0 overflow-hidden">
												<Item.Title class="block w-full font-mono text-sm truncate text-left">{cred.alias}</Item.Title>
											</Item.Content>
											<Item.Actions class="shrink-0 ml-auto">
												<Badge variant="outline" class="truncate {policyColor[cred.policy] ?? ''}">{policyLabel[cred.policy] ?? cred.policy}</Badge>
											</Item.Actions>
										</button>
									{/snippet}
								</Item.Root>
								{#if index !== credentialList.length - 1}
									<Item.Separator />
								{/if}
							{/each}
						</Item.Group>
					{/if}
				</div>
			</div>
		</Card.Root>

		{#if currentProjectSlug && !interruptMode}
			<NotesCard slug={currentProjectSlug} refresh={notesRefresh} />
		{/if}

		<!-- Footer -->
		<div class="mt-8 flex w-full max-w-4xl items-center justify-between px-8">
			<div class="flex items-center gap-2">
				<Button variant="outline" size="sm" onclick={() => (drawerOpen = true)}>All Projects</Button>
			</div>
			<div class="flex flex-col items-end gap-2 text-muted-foreground text-xs">
				<div class="flex items-center gap-1.5 text-foreground"><svg viewBox="0 0 96 96" class="size-[18px]" fill="none" aria-hidden="true"><g stroke="#E11D48" stroke-width="7" stroke-linecap="round"><line x1="48" y1="48" x2="80" y2="48"/><line x1="48" y1="48" x2="70.63" y2="70.63"/><line x1="48" y1="48" x2="48" y2="80"/><line x1="48" y1="48" x2="25.37" y2="70.63"/><line x1="48" y1="48" x2="16" y2="48"/><line x1="48" y1="48" x2="25.37" y2="25.37"/><line x1="48" y1="48" x2="48" y2="16"/><line x1="48" y1="48" x2="70.63" y2="25.37"/><line x1="48" y1="48" x2="72.02" y2="57.95"/><line x1="48" y1="48" x2="57.95" y2="72.02"/><line x1="48" y1="48" x2="38.05" y2="72.02"/><line x1="48" y1="48" x2="23.98" y2="57.95"/><line x1="48" y1="48" x2="23.98" y2="38.05"/><line x1="48" y1="48" x2="38.05" y2="23.98"/><line x1="48" y1="48" x2="57.95" y2="23.98"/><line x1="48" y1="48" x2="72.02" y2="38.05"/></g><circle cx="48" cy="48" r="14" fill="currentColor"/></svg><span class="font-bold leading-none" style="font-family:'Outfit',ui-sans-serif,sans-serif">yucca</span></div>
			</div>
		</div>

		<ProjectPicker
			bind:open={drawerOpen}
			currentProjectSlug={currentProjectSlug}
			mainSelectedAlias={selectedAlias}
			onselect={(project) => {
				selectProject(project.slug);
			}}
			oncopy={() => {
				if (currentProjectSlug) loadCredentialsForProject(currentProjectSlug);
			}}
			oncreate={(data) => {
				const namePart = data.fromAlias.includes(':') ? data.fromAlias.split(':').pop() : data.fromAlias;
				const projectPrefix = data.fromProjectName.toLowerCase().replace(/\s+/g, '-').replace(/[^A-Za-z0-9_\-.]/g, '');
				const sanitized = `${projectPrefix}-${namePart}`.replace(/[^A-Za-z0-9_\-.]/g, '').slice(0, 64);
				enterCreateMode({
					alias: sanitized,
					policy: data.policy,
					context: data.context,
					copyFrom: { projectSlug: data.fromProjectSlug, alias: data.fromAlias, projectName: data.fromProjectName },
				});
			}}
			onpastevalue={(data) => {
				applyPasteValue(data);
			}}
		/>

		{/if}
	{/if}
</div>
