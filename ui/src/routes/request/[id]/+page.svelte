<script lang="ts">
	import { page } from '$app/stores';
	import { onMount } from 'svelte';
	import { fade } from 'svelte/transition';
	import {
		fetchRequest,
		approveRequest,
		denyRequest,
	} from '$lib/api';
	import type { ApprovalPolicy, SecretRequest } from '$lib/api';
	import * as Card from '$lib/components/ui/card';
	import * as RadioGroup from '$lib/components/ui/radio-group';
	import * as InputGroup from '$lib/components/ui/input-group';
	import { Button } from '$lib/components/ui/button';
	import { Label } from '$lib/components/ui/label';
	import { Separator } from '$lib/components/ui/separator';
	import KeyRoundIcon from '@lucide/svelte/icons/key-round';
	import TerminalIcon from '@lucide/svelte/icons/terminal';
	import ShieldAlertIcon from '@lucide/svelte/icons/shield-alert';
	import CheckIcon from '@lucide/svelte/icons/check';
	import XIcon from '@lucide/svelte/icons/x';

	const id = $derived($page.params.id ?? '');

	let request = $state<SecretRequest | null>(null);
	let loading = $state(true);
	let resolved = $state(false);
	let resolvedStatus = $state('');
	let secretValue = $state('');
	let policy: ApprovalPolicy = $state('ask_session');
	let ws: WebSocket | null = null;

	let isExec = $derived(request?.kind === 'execute_accept');
	let execCommand = $derived(
		isExec && request?.reason?.startsWith('exec: ')
			? request!.reason.slice(6)
			: request?.reason ?? ''
	);

	onMount(() => {
		loadRequest();
		connectWebSocket();
		return () => ws?.close();
	});

	async function loadRequest() {
		try {
			request = await fetchRequest(id);
			if (request.status !== 'pending') {
				resolved = true;
				resolvedStatus = request.status;
			}
		} catch {
			// request not found
		} finally {
			loading = false;
		}
	}

	function closeWindow() {
		// The native tray app closes its approval window on request_resolved;
		// in a real browser tab this closes it best-effort.
		window.close();
	}

	function connectWebSocket() {
		const wsURL = `ws://${window.location.host}/api/ws`;
		ws = new WebSocket(wsURL);
		ws.onmessage = (event) => {
			const msg = JSON.parse(event.data);
			if (msg.type === 'request_resolved' && msg.data?.id === id) {
				resolved = true;
				resolvedStatus = msg.data.status ?? 'resolved';
				closeWindow();
			}
		};
		ws.onclose = () => {
			if (!resolved) setTimeout(connectWebSocket, 2000);
		};
	}

	async function handleApprove() {
		if (!request) return;
		await approveRequest(request.id, secretValue, policy);
		resolved = true;
		resolvedStatus = 'approved';
	}

	async function handleDeny() {
		if (!request) return;
		await denyRequest(request.id);
		resolved = true;
		resolvedStatus = 'denied';
	}
</script>

<svelte:head>
	<style>
		html, body {
			overflow: hidden !important;
			height: 100% !important;
		}
	</style>
</svelte:head>

<div class="flex h-screen items-center justify-center p-6 overflow-hidden">
	{#if loading}
		<p class="text-muted-foreground text-sm">Loading...</p>
	{:else if resolved}
		<div in:fade={{ duration: 200 }}>
			<Card.Root class="w-full max-w-md p-8">
				<div class="flex flex-col items-center gap-4 text-center">
					{#if resolvedStatus === 'approved'}
						<CheckIcon class="size-10 text-primary" />
						<h3 class="text-lg font-medium">Approved</h3>
					{:else}
						<XIcon class="size-10 text-destructive" />
						<h3 class="text-lg font-medium">Denied</h3>
					{/if}
				</div>
			</Card.Root>
		</div>
	{:else if request}
		<Card.Root class="w-full max-w-md p-6">
			{#if isExec}
				<!-- Execute accept -->
				<div class="flex items-center gap-2 mb-4">
					<TerminalIcon class="size-5 text-primary" />
					<div>
						<small class="text-sm leading-none font-medium">Command Execution</small>
						{#if request.project_name}
							<p class="text-xs text-muted-foreground">{request.project_name}</p>
						{/if}
					</div>
				</div>

				<Separator class="mb-4" />

				<div class="flex flex-col gap-4">
					<div>
						<small class="text-sm leading-none font-medium">Command</small>
						<pre class="mt-1 rounded-md bg-muted p-3 text-sm font-mono whitespace-pre-wrap break-all max-h-24 overflow-y-auto">{execCommand}</pre>
					</div>

					<Separator />

					<div>
						<small class="text-sm leading-none font-medium">Credentials ({request.aliases?.length ?? 0})</small>
						<div class="mt-2 flex flex-col gap-1 max-h-32 overflow-y-auto">
							{#each request.aliases ?? [] as alias}
								<div class="flex items-center gap-2 rounded-md bg-muted/50 px-3 py-2">
									<ShieldAlertIcon class="size-4 text-muted-foreground shrink-0" />
									<code class="text-sm font-mono">{alias}</code>
								</div>
							{/each}
						</div>
					</div>

					<div class="flex gap-2 pt-2">
						<Button variant="secondary" class="flex-1" onclick={handleApprove}>
							Approve
						</Button>
						<Button variant="destructive" class="flex-1" onclick={handleDeny}>
							Deny
						</Button>
					</div>
				</div>
			{:else}
				<!-- Secret request -->
				<div class="flex items-center gap-2 mb-4">
					<KeyRoundIcon class="size-5 text-primary" />
					<div>
						<small class="text-sm leading-none font-medium">Secret Request</small>
						{#if request.project_name}
							<p class="text-xs text-muted-foreground">{request.project_name}</p>
						{/if}
						<p class="font-mono leading-7">{request.alias}</p>
					</div>
				</div>

				<Separator class="mb-4" />

				<div class="space-y-4">
					{#if request.reason}
						<div>
							<small class="text-sm leading-none font-medium">Reason</small>
							<p class="text-muted-foreground leading-7">"{request.reason}"</p>
						</div>
						<Separator />
					{/if}

					<div class="space-y-2">
						<Label for="secret-val">Secret</Label>
						<InputGroup.Root>
							<InputGroup.Input
								id="secret-val"
								type="password"
								placeholder="Enter secret value"
								bind:value={secretValue}
							/>
						</InputGroup.Root>
					</div>

					<div class="space-y-2">
						<Label>Policy</Label>
						<RadioGroup.Root bind:value={policy}>
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

					<div class="flex gap-2 pt-2">
						<Button variant="secondary" class="flex-1" onclick={handleApprove}>
							Approve
						</Button>
						<Button variant="destructive" class="flex-1" onclick={handleDeny}>
							Deny
						</Button>
					</div>
				</div>
			{/if}
		</Card.Root>
	{:else}
		<Card.Root class="w-full max-w-md p-8">
			<div class="flex flex-col items-center gap-4 text-center">
				<p class="text-muted-foreground text-sm">Request not found</p>
			</div>
		</Card.Root>
	{/if}
</div>
