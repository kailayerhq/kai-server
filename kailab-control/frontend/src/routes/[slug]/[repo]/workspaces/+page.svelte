<script>
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/stores';
	import { api, loadUser } from '$lib/api.js';
	import RepoNav from '$lib/components/RepoNav.svelte';

	let workspaces = $state([]);
	let loading = $state(true);
	let error = $state('');

	onMount(async () => {
		const user = await loadUser();
		if (!user) {
			goto('/login');
			return;
		}
		await loadWorkspaces();
	});

	async function loadWorkspaces() {
		loading = true;
		error = '';
		const { slug, repo } = $page.params;

		try {
			const data = await api('GET', `/${slug}/${repo}/v1/refs?prefix=ws.`);
			if (data.error) {
				error = data.error;
				workspaces = [];
			} else {
				workspaces = (data.refs || []).map(ref => ({
					name: ref.name.replace(/^ws\./, ''),
					ref: ref.name,
					digest: ref.target ? hexEncode(ref.target) : '',
				}));
			}
		} catch (e) {
			error = 'Failed to load workspaces';
			workspaces = [];
		}

		loading = false;
	}

	function hexEncode(bytes) {
		if (!bytes) return '';
		try {
			const decoded = atob(bytes);
			return Array.from(decoded, (c) => c.charCodeAt(0).toString(16).padStart(2, '0')).join('');
		} catch {
			return bytes;
		}
	}

	function shortHex(hex) {
		if (!hex) return '';
		return hex.slice(0, 8);
	}

	function viewWorkspace(ws) {
		const { slug, repo } = $page.params;
		goto(`/${slug}/${repo}/ws.${ws.name}`);
	}
</script>

<div class="max-w-6xl mx-auto px-5 py-8">
	<RepoNav active="workspaces" />

	{#if loading}
		<div class="text-center py-12 text-kai-text-muted">Loading...</div>
	{:else if error}
		<div class="card text-center py-12">
			<p class="text-red-700 dark:text-red-400 mb-4">{error}</p>
			<button class="btn" onclick={loadWorkspaces}>Retry</button>
		</div>
	{:else if workspaces.length === 0}
		<div class="card text-center py-12">
			<p class="text-kai-text-muted mb-4">No workspaces yet</p>
			<p class="text-kai-text-muted text-sm">
				Create a workspace with <code class="bg-kai-bg-tertiary px-2 py-1 rounded">kai ws checkout &lt;name&gt;</code>
			</p>
		</div>
	{:else}
		<div class="card p-0">
			{#each workspaces as ws}
				<button
					class="list-item w-full text-left hover:bg-kai-bg-tertiary transition-colors cursor-pointer"
					onclick={() => viewWorkspace(ws)}
				>
					<div class="flex-1 min-w-0">
						<div class="flex items-center gap-3">
							<svg class="w-4 h-4 text-kai-text-muted shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
								<path stroke-linecap="round" stroke-linejoin="round" d="M6 3v12m0 0a3 3 0 1 0 3 3m-3-3a3 3 0 0 1 3 3m0 0h6m0 0a3 3 0 1 0 3-3m-3 3a3 3 0 0 1 3-3m0 0V9m0 0a3 3 0 1 0-3-3m3 3a3 3 0 0 1-3-3m0 0H9" />
							</svg>
							<span class="font-medium truncate">{ws.name}</span>
						</div>
						{#if ws.digest}
							<div class="text-kai-text-muted text-xs mt-1 ml-7">
								<span class="font-mono">{shortHex(ws.digest)}</span>
							</div>
						{/if}
					</div>
					<div class="text-kai-text-muted">
						<svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
							<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7" />
						</svg>
					</div>
				</button>
			{/each}
		</div>
	{/if}
</div>
