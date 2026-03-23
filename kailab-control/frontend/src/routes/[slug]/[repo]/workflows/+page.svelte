<script>
	import { onMount, onDestroy } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/stores';
	import { api, loadUser } from '$lib/api.js';
	import RepoNav from '$lib/components/RepoNav.svelte';

	let runs = $state([]);
	let workflows = $state([]);
	let loading = $state(true);
	let error = $state('');
	let eventSource = $state(null);
	let currentPage = $state(1);
	let totalRuns = $state(0);
	let pageSize = 20;
	let ciAccessDenied = $state(false);
	let ciRequested = $state(false);
	let ciRequesting = $state(false);

	async function requestCIAccess() {
		ciRequesting = true;
		const data = await api('POST', '/api/v1/ci/request-access');
		if (data.status === 'requested' || data.status === 'already_requested') {
			ciRequested = true;
		}
		ciRequesting = false;
	}

	$effect(() => {
		$page.params.slug;
		$page.params.repo;
	});

	let destroyed = false;

	onMount(async () => {
		const user = await loadUser();
		if (!user) {
			goto('/login');
			return;
		}
		if (!user.ci_access) {
			ciAccessDenied = true;
			ciRequested = user.ci_requested || false;
			loading = false;
			return;
		}
		// Read page from URL
		const urlPage = new URL(window.location).searchParams.get('page');
		if (urlPage) currentPage = parseInt(urlPage) || 1;
		await loadData();
		connectSSE();
	});

	onDestroy(() => {
		destroyed = true;
		if (eventSource) {
			eventSource.close();
			eventSource = null;
		}
	});

	function connectSSE() {
		if (destroyed) return;
		const { slug, repo } = $page.params;
		const es = new EventSource(`/api/v1/orgs/${slug}/repos/${repo}/runs/events`, { withCredentials: true });

		es.addEventListener('runs', (e) => {
			try {
				const data = JSON.parse(e.data);
				if (data.runs) {
					runs = data.runs;
				}
			} catch {}
		});

		es.onerror = () => {
			es.close();
			if (!destroyed) {
				setTimeout(connectSSE, 5000);
			}
		};

		eventSource = es;
	}

	async function loadData() {
		loading = true;
		error = '';
		const { slug, repo } = $page.params;

		try {
			const [runsData, workflowsData] = await Promise.all([
				api('GET', `/api/v1/orgs/${slug}/repos/${repo}/runs?limit=${pageSize}&page=${currentPage}`),
				api('GET', `/api/v1/orgs/${slug}/repos/${repo}/workflows`)
			]);

			runs = runsData?.runs || [];
			totalRuns = runsData?.total || 0;
			workflows = workflowsData?.workflows || [];

			if (workflows.length === 0 && runs.length === 0) {
				const discovered = await api('POST', `/api/v1/orgs/${slug}/repos/${repo}/workflows/discover`);
				workflows = discovered?.workflows || [];
			}
		} catch (e) {
			error = 'Failed to load workflow data';
		}

		loading = false;
	}

	async function goToPage(p) {
		currentPage = p;
		const { slug, repo } = $page.params;
		// Update URL without navigation
		const url = new URL(window.location);
		if (p > 1) {
			url.searchParams.set('page', p);
		} else {
			url.searchParams.delete('page');
		}
		window.history.replaceState({}, '', url);
		try {
			const data = await api('GET', `/api/v1/orgs/${slug}/repos/${repo}/runs?limit=${pageSize}&page=${p}`);
			runs = data?.runs || [];
			totalRuns = data?.total || 0;
		} catch {}
	}

	let totalPages = $derived(Math.ceil(totalRuns / pageSize));

	function getStatusColor(status, conclusion) {
		if (status === 'completed') {
			switch (conclusion) {
				case 'success':
					return 'bg-green-600/10 dark:bg-green-500/20 text-green-700 dark:text-green-400';
				case 'failure':
					return 'bg-red-600/10 dark:bg-red-500/20 text-red-700 dark:text-red-400';
				case 'cancelled':
					return 'bg-gray-500/10 dark:bg-gray-500/20 text-gray-600 dark:text-gray-400';
				default:
					return 'bg-gray-500/10 dark:bg-gray-500/20 text-gray-600 dark:text-gray-400';
			}
		}
		switch (status) {
			case 'queued':
				return 'bg-yellow-600/10 dark:bg-yellow-500/20 text-yellow-700 dark:text-yellow-400';
			case 'in_progress':
				return 'bg-blue-600/10 dark:bg-blue-500/20 text-blue-700 dark:text-blue-400';
			default:
				return 'bg-gray-500/10 dark:bg-gray-500/20 text-gray-600 dark:text-gray-400';
		}
	}

	function getStatusIcon(status, conclusion) {
		if (status === 'completed') {
			switch (conclusion) {
				case 'success':
					return '✓';
				case 'failure':
					return '✕';
				case 'cancelled':
					return '⊘';
				default:
					return '?';
			}
		}
		switch (status) {
			case 'queued':
				return '◦';
			case 'in_progress':
				return '●';
			default:
				return '?';
		}
	}

	function formatStatus(status, conclusion) {
		if (status === 'completed') {
			return conclusion.charAt(0).toUpperCase() + conclusion.slice(1);
		}
		return status.replace(/_/g, ' ').replace(/\b\w/g, c => c.toUpperCase());
	}

	function formatDate(timestamp) {
		if (!timestamp) return '';
		return new Date(timestamp).toLocaleDateString('en-US', {
			month: 'short',
			day: 'numeric',
			hour: '2-digit',
			minute: '2-digit'
		});
	}

	function formatDuration(startedAt, completedAt) {
		if (!startedAt) return '';
		const start = new Date(startedAt);
		const end = completedAt ? new Date(completedAt) : new Date();
		const diff = Math.floor((end - start) / 1000);

		if (diff < 60) return `${diff}s`;
		if (diff < 3600) return `${Math.floor(diff / 60)}m ${diff % 60}s`;
		return `${Math.floor(diff / 3600)}h ${Math.floor((diff % 3600) / 60)}m`;
	}

	function getRefDisplay(ref) {
		if (!ref) return '';
		if (ref.startsWith('refs/heads/')) {
			return ref.replace('refs/heads/', '');
		}
		if (ref.startsWith('refs/tags/')) {
			return ref.replace('refs/tags/', '');
		}
		return ref;
	}

	function viewRun(run) {
		const { slug, repo } = $page.params;
		goto(`/${slug}/${repo}/workflows/runs/${run.id}`);
	}

	function getWorkflowFileName(path) {
		return path.split('/').pop() || path;
	}
</script>

<div class="max-w-6xl mx-auto px-5 py-8">
	<RepoNav active="ci" />

	{#if ciAccessDenied}
		<div class="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
			<div class="bg-kai-bg-secondary border border-kai-border rounded-lg p-8 max-w-md mx-4 shadow-xl">
				<div class="flex items-center gap-3 mb-4">
					<div class="w-10 h-10 rounded-full bg-kai-accent/10 flex items-center justify-center">
						<svg class="w-5 h-5 text-kai-accent" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
							<path stroke-linecap="round" stroke-linejoin="round" d="M9.75 17L9 20l-1 1h8l-1-1-.75-3M3 13h18M5 17h14a2 2 0 002-2V5a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z" />
						</svg>
					</div>
					<h2 class="text-lg font-semibold text-kai-text">CI is in Early Beta</h2>
				</div>
				<p class="text-kai-text-muted text-sm leading-relaxed mb-6">
					Kailab CI is currently available by request. It includes workflow automation, test orchestration, and deployment pipelines — all powered by your semantic code graph.
				</p>
				<div class="flex gap-3">
					{#if ciRequested}
						<span class="btn btn-primary text-sm opacity-75 cursor-default">Access Requested</span>
					{:else}
						<button class="btn btn-primary text-sm" onclick={requestCIAccess} disabled={ciRequesting}>
							{ciRequesting ? 'Requesting...' : 'Request Access'}
						</button>
					{/if}
					<button class="btn text-sm" onclick={() => history.back()}>Go Back</button>
				</div>
			</div>
		</div>
	{:else if loading}
		<div class="space-y-3 animate-pulse">
			{#each Array(5) as _}
				<div class="border border-kai-border rounded-md p-4 bg-kai-bg-secondary">
					<div class="flex items-center gap-3">
						<div class="w-6 h-6 bg-kai-bg-tertiary rounded-full"></div>
						<div class="flex-1">
							<div class="h-4 bg-kai-bg-tertiary rounded w-1/3 mb-2"></div>
							<div class="h-3 bg-kai-bg-tertiary rounded w-1/4"></div>
						</div>
						<div class="h-4 bg-kai-bg-tertiary rounded w-16"></div>
					</div>
				</div>
			{/each}
		</div>
	{:else if error}
		<div class="card text-center py-12">
			<p class="text-red-700 dark:text-red-400 mb-4">{error}</p>
			<button class="btn" onclick={() => loadData()}>Retry</button>
		</div>
	{:else if runs.length === 0 && workflows.length === 0}
		<div class="card text-center py-12">
			<p class="text-kai-text-muted mb-4">No workflow runs yet</p>
			<p class="text-kai-text-muted text-sm">
				Create workflows in <code class="bg-kai-bg-tertiary px-2 py-1 rounded">.kailab/workflows/</code> to get started
			</p>
		</div>
	{:else}
		{#if workflows.length > 0 && runs.length === 0}
			<div class="card mb-6">
				<div class="px-4 py-3 border-b border-kai-border">
					<h3 class="font-medium text-sm">Workflows</h3>
				</div>
				{#each workflows as wf}
					<div class="list-item">
						<div class="flex items-center gap-3 min-w-0 flex-1">
							<svg class="w-4 h-4 text-kai-text-muted flex-shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24">
								<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z" />
							</svg>
							<div class="min-w-0">
								<div class="font-medium">{wf.name}</div>
								<div class="text-kai-text-muted text-xs mt-0.5">
									<span class="font-mono">{getWorkflowFileName(wf.path)}</span>
									{#if wf.triggers?.length}
										<span class="mx-1.5">·</span>
										{#each wf.triggers as trigger, i}
											<span class="px-1.5 py-0.5 rounded bg-kai-bg-tertiary">{trigger}</span>
											{#if i < wf.triggers.length - 1}<span class="mx-0.5"></span>{/if}
										{/each}
									{/if}
								</div>
							</div>
						</div>
					</div>
				{/each}
				<div class="px-4 py-3 text-kai-text-muted text-sm border-t border-kai-border">
					No runs yet. Push to the repository to trigger workflows.
				</div>
			</div>
		{/if}

		{#if runs.length > 0}
			<div class="text-sm text-kai-text-muted mb-3">{totalRuns} workflow runs</div>
			<div class="border border-kai-border rounded-lg overflow-hidden">
				{#each runs as run}
					<button
						class="w-full text-left px-4 py-3 border-b border-kai-border last:border-b-0 transition-colors cursor-pointer hover:bg-kai-bg-tertiary/50
							{run.conclusion === 'failure' ? 'border-l-4 border-l-red-500 pl-[12px] bg-red-500/[0.03] dark:bg-red-500/[0.05]' : 'border-l-4 border-l-transparent pl-[12px]'}
							{run.status === 'in_progress' ? 'border-l-blue-500' : ''}"
						onclick={() => viewRun(run)}
					>
						<div class="flex items-start gap-3">
							<span class="w-5 h-5 rounded-full flex items-center justify-center text-xs font-bold mt-0.5 shrink-0
								{run.conclusion === 'success' ? 'bg-green-600/15 text-green-600 dark:text-green-400' :
								 run.conclusion === 'failure' ? 'bg-red-600/15 text-red-600 dark:text-red-400' :
								 run.status === 'in_progress' ? 'bg-blue-600/15 text-blue-600 dark:text-blue-400 animate-pulse' :
								 run.status === 'queued' ? 'bg-yellow-600/15 text-yellow-600 dark:text-yellow-400' :
								 'bg-gray-500/15 text-gray-500'}">
								{getStatusIcon(run.status, run.conclusion)}
							</span>
							<div class="flex-1 min-w-0">
								<div class="font-semibold text-sm truncate">
									{run.trigger_message || run.workflow_name || 'Workflow run'}
								</div>
								<div class="text-kai-text-muted text-xs mt-0.5 flex items-center gap-2 flex-wrap">
									<span>{run.workflow_name} #{run.run_number}</span>
									<span class="text-kai-text-muted/50">·</span>
									<span class="px-1.5 py-0.5 rounded bg-kai-bg-tertiary">{getRefDisplay(run.trigger_ref) || run.trigger_event}</span>
									{#if run.trigger_sha}
										<code class="font-mono text-[11px]">{run.trigger_sha.slice(0, 7)}</code>
									{/if}
									{#if run.trigger_actor}
										<span class="text-kai-text-muted/50">·</span>
										<span>{run.trigger_actor}</span>
									{/if}
								</div>
							</div>
							<div class="text-right shrink-0">
								<div class="text-xs tabular-nums text-kai-text-muted">
									{#if run.started_at}
										{formatDuration(run.started_at, run.completed_at)}
									{/if}
								</div>
								<div class="text-xs tabular-nums text-kai-text-muted mt-0.5">
									{#if run.created_at}
										{formatDate(run.created_at)}
									{/if}
								</div>
							</div>
						</div>
					</button>
				{/each}
			</div>

			<!-- Pagination -->
			{#if totalPages > 1}
				<div class="flex items-center justify-between mt-4">
					<div class="text-sm text-kai-text-muted">
						{totalRuns} runs
					</div>
					<div class="flex items-center gap-1">
						<button
							class="btn text-xs"
							disabled={currentPage === 1}
							onclick={() => goToPage(currentPage - 1)}
						>
							Previous
						</button>
						{#each Array(Math.min(totalPages, 5)) as _, i}
							{@const p = currentPage <= 3 ? i + 1 : currentPage - 2 + i}
							{#if p >= 1 && p <= totalPages}
								<button
									class="px-2 py-1 text-xs rounded {p === currentPage ? 'bg-kai-accent text-white' : 'text-kai-text-muted hover:text-kai-text'}"
									onclick={() => goToPage(p)}
								>
									{p}
								</button>
							{/if}
						{/each}
						<button
							class="btn text-xs"
							disabled={currentPage === totalPages}
							onclick={() => goToPage(currentPage + 1)}
						>
							Next
						</button>
					</div>
				</div>
			{/if}
		{/if}
	{/if}
</div>
