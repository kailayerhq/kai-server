<script>
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/stores';
	import { api, loadUser } from '$lib/api.js';

	const ANSI_COLORS = {
		'30': '#6c7086', '31': '#f38ba8', '32': '#a6e3a1', '33': '#f9e2af',
		'34': '#89b4fa', '35': '#cba6f7', '36': '#94e2d5', '37': '#cdd6f4',
		'90': '#6c7086', '91': '#f38ba8', '92': '#a6e3a1', '93': '#f9e2af',
		'94': '#89b4fa', '95': '#cba6f7', '96': '#94e2d5', '97': '#cdd6f4',
	};

	function ansiToHtml(text) {
		// Escape HTML first
		let s = text.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
		let result = '';
		let openSpans = 0;
		const parts = s.split(/\x1b\[/);
		result += parts[0];
		for (let i = 1; i < parts.length; i++) {
			const m = parts[i].match(/^([0-9;]*)([a-zA-Z])(.*)/s);
			if (!m) { result += parts[i]; continue; }
			const [, codes, letter, rest] = m;
			if (letter !== 'm') { result += rest; continue; }
			const nums = codes.split(';').filter(Boolean);
			if (nums.length === 0 || nums[0] === '0') {
				while (openSpans > 0) { result += '</span>'; openSpans--; }
			} else {
				for (const n of nums) {
					const color = ANSI_COLORS[n];
					if (color) {
						result += `<span style="color:${color}">`;
						openSpans++;
					} else if (n === '1') {
						result += '<span style="font-weight:bold">';
						openSpans++;
					}
				}
			}
			result += rest;
		}
		while (openSpans > 0) { result += '</span>'; openSpans--; }
		return result;
	}

	let run = $state(null);
	let jobs = $state([]);
	let selectedJob = $state(null);
	let logs = $state([]);
	let loading = $state(true);
	let logsLoading = $state(false);
	let error = $state('');
	let pollInterval = $state(null);
	let lastLogSeq = $state(-1);
	let logContainer = $state(null);

	onMount(async () => {
		const user = await loadUser();
		if (!user) { goto('/login'); return; }
		await loadRun();
		startPolling();
		return () => { if (pollInterval) clearInterval(pollInterval); };
	});

	function startPolling() {
		pollInterval = setInterval(async () => {
			if (run && (run.status === 'queued' || run.status === 'in_progress')) {
				await loadRun(true);
				if (selectedJob) await loadLogs(true);
			}
		}, 3000);
	}

	async function loadRun(silent = false) {
		if (!silent) { loading = true; error = ''; }
		const { slug, repo, id } = $page.params;
		try {
			const [runData, jobsData] = await Promise.all([
				api('GET', `/api/v1/orgs/${slug}/repos/${repo}/runs/${id}`),
				api('GET', `/api/v1/orgs/${slug}/repos/${repo}/runs/${id}/jobs`)
			]);
			if (runData.error) { error = runData.error; return; }
			run = runData;
			jobs = jobsData.jobs || [];
			if (!selectedJob && jobs.length > 0) {
				// Auto-select first failed job, or first job
				const failed = jobs.find(j => j.conclusion === 'failure');
				selectJob(failed || jobs[0]);
			}
		} catch (e) { if (!silent) error = 'Failed to load workflow run'; }
		if (!silent) loading = false;
	}

	async function selectJob(job) {
		selectedJob = job;
		lastLogSeq = -1;
		logs = [];
		await loadLogs();
	}

	async function loadLogs(append = false) {
		if (!selectedJob) return;
		logsLoading = !append;
		const { slug, repo, id } = $page.params;
		try {
			const url = append && lastLogSeq >= 0
				? `/api/v1/orgs/${slug}/repos/${repo}/runs/${id}/jobs/${selectedJob.id}/logs?after=${lastLogSeq}`
				: `/api/v1/orgs/${slug}/repos/${repo}/runs/${id}/jobs/${selectedJob.id}/logs`;
			const data = await api('GET', url);
			if (data.logs && data.logs.length > 0) {
				logs = append ? [...logs, ...data.logs] : data.logs;
				lastLogSeq = data.logs[data.logs.length - 1].chunk_seq;
				// Auto-scroll to bottom after logs update
				requestAnimationFrame(() => {
					if (logContainer) {
						logContainer.scrollTop = logContainer.scrollHeight;
					}
				});
			}
		} catch (e) {}
		logsLoading = false;
	}

	async function cancelRun() {
		const { slug, repo, id } = $page.params;
		try { await api('POST', `/api/v1/orgs/${slug}/repos/${repo}/runs/${id}/cancel`); await loadRun(); }
		catch (e) { error = 'Failed to cancel run'; }
	}

	async function rerunWorkflow() {
		const { slug, repo, id } = $page.params;
		try {
			const data = await api('POST', `/api/v1/orgs/${slug}/repos/${repo}/runs/${id}/rerun`);
			if (data?.id) {
				goto(`/${slug}/${repo}/workflows/runs/${data.id}`);
			} else {
				goto(`/${slug}/${repo}/workflows`);
			}
		} catch (e) {
			error = 'Failed to re-run workflow';
		}
	}

	function jumpToError() {
		if (!logContainer) return;
		const errorLine = logContainer.querySelector('.log-error, .log-failure');
		if (errorLine) errorLine.scrollIntoView({ behavior: 'smooth', block: 'center' });
	}

	// Parse log content into classified lines
	function parseLogLines(logEntries) {
		const lines = [];
		let lineNum = 1;
		for (const entry of logEntries) {
			for (const raw of entry.content.split('\n')) {
				const line = { num: lineNum++, text: raw, type: 'default' };
				if (raw.startsWith('=== ') && raw.endsWith(' ===')) {
					line.type = 'section';
				} else if (raw.includes('Step completed: success') || raw.startsWith('ok ')) {
					line.type = 'success';
				} else if (raw.includes('Step completed: failure') || raw.startsWith('FAIL') || raw.includes('Exit code:') || raw.includes('error:') || raw.includes('Error:')) {
					line.type = 'error';
				} else if (raw.startsWith('---') || raw.startsWith('===')) {
					line.type = 'section';
				} else if (raw.startsWith('#') || raw.includes('missing go.sum') || raw.includes('[setup failed]')) {
					line.type = 'error';
				} else if (raw.startsWith('Downloading') || raw.startsWith('Extracting') || raw.startsWith('Checkout') || raw.startsWith('Fetching') || raw.startsWith('Installing')) {
					line.type = 'info';
				} else if (raw.startsWith('PASS') || raw.includes('determinism verified')) {
					line.type = 'success';
				} else if (raw.startsWith('---') && raw.includes('FAIL')) {
					line.type = 'error';
				} else if (raw.startsWith('go: downloading')) {
					line.type = 'muted';
				}
				lines.push(line);
			}
		}
		return lines;
	}

	let parsedLines = $derived(parseLogLines(logs));
	let hasError = $derived(parsedLines.some(l => l.type === 'error'));

	function getStatusColor(status, conclusion) {
		if (status === 'completed') {
			switch (conclusion) {
				case 'success': return 'text-green-600 dark:text-green-400';
				case 'failure': return 'text-red-600 dark:text-red-400';
				default: return 'text-kai-text-muted';
			}
		}
		if (status === 'in_progress') return 'text-blue-600 dark:text-blue-400';
		if (status === 'queued') return 'text-yellow-600 dark:text-yellow-400';
		return 'text-kai-text-muted';
	}

	function getStatusBadge(status, conclusion) {
		if (status === 'completed') {
			switch (conclusion) {
				case 'success': return 'bg-green-600/10 text-green-700 dark:text-green-400 border border-green-600/20';
				case 'failure': return 'bg-red-600/10 text-red-700 dark:text-red-400 border border-red-600/20';
				default: return 'bg-gray-500/10 text-gray-600 dark:text-gray-400 border border-gray-500/20';
			}
		}
		if (status === 'in_progress') return 'bg-blue-600/10 text-blue-700 dark:text-blue-400 border border-blue-600/20';
		if (status === 'queued') return 'bg-yellow-600/10 text-yellow-700 dark:text-yellow-400 border border-yellow-600/20';
		return 'bg-gray-500/10 text-gray-600 dark:text-gray-400 border border-gray-500/20';
	}

	function getStatusIcon(status, conclusion) {
		if (status === 'completed') {
			switch (conclusion) {
				case 'success': return '✓';
				case 'failure': return '✕';
				default: return '—';
			}
		}
		if (status === 'in_progress') return '●';
		if (status === 'queued') return '◦';
		return '○';
	}

	function formatStatus(status, conclusion) {
		if (status === 'completed') return conclusion.charAt(0).toUpperCase() + conclusion.slice(1);
		return status.replace(/_/g, ' ').replace(/\b\w/g, c => c.toUpperCase());
	}

	function formatDuration(startedAt, completedAt) {
		if (!startedAt) return '—';
		const start = new Date(startedAt);
		const end = completedAt ? new Date(completedAt) : new Date();
		const diff = Math.floor((end - start) / 1000);
		if (diff < 60) return `${diff}s`;
		if (diff < 3600) return `${Math.floor(diff / 60)}m ${diff % 60}s`;
		return `${Math.floor(diff / 3600)}h ${Math.floor((diff % 3600) / 60)}m`;
	}

	function formatDate(ts) {
		if (!ts) return '';
		return new Date(ts).toLocaleString('en-US', { month: 'short', day: 'numeric', year: 'numeric', hour: '2-digit', minute: '2-digit' });
	}

	function getRefDisplay(ref) {
		if (!ref) return '';
		return ref.replace('refs/heads/', '').replace('refs/tags/', '');
	}
</script>

<div class="max-w-7xl mx-auto px-5 py-8">
	{#if loading}
		<div class="space-y-4 animate-pulse">
			<div class="h-4 bg-kai-bg-tertiary rounded w-1/3"></div>
			<div class="h-6 bg-kai-bg-tertiary rounded w-1/2"></div>
			<div class="grid grid-cols-12 gap-6 mt-6">
				<div class="col-span-3 space-y-2">
					{#each Array(3) as _}
						<div class="h-16 bg-kai-bg-tertiary rounded"></div>
					{/each}
				</div>
				<div class="col-span-9 h-96 bg-kai-bg-tertiary rounded"></div>
			</div>
		</div>
	{:else if error}
		<div class="card text-center py-12">
			<p class="text-red-700 dark:text-red-400 mb-4">{error}</p>
			<button class="btn" onclick={() => loadRun()}>Retry</button>
		</div>
	{:else if run}
		<!-- Header -->
		<div class="mb-6">
			<nav class="text-sm text-kai-text-muted mb-3 flex items-center gap-1.5">
				<a href="/{$page.params.slug}" class="hover:text-kai-text no-underline">{$page.params.slug}</a>
				<span>/</span>
				<a href="/{$page.params.slug}/{$page.params.repo}" class="hover:text-kai-text no-underline font-medium text-kai-text">{$page.params.repo}</a>
				<span>/</span>
				<a href="/{$page.params.slug}/{$page.params.repo}/workflows" class="hover:text-kai-text no-underline">CI</a>
				<span>/</span>
				<span class="text-kai-text">#{run.run_number}</span>
			</nav>

			<div class="flex items-start justify-between">
				<div>
					<div class="flex items-center gap-3">
						<h2 class="text-xl font-semibold">{run.workflow_name || 'Workflow'} <span class="text-kai-text-muted font-normal">#{run.run_number}</span></h2>
						<span class="px-2.5 py-1 rounded-md text-sm font-medium {getStatusBadge(run.status, run.conclusion)}">
							{getStatusIcon(run.status, run.conclusion)} {formatStatus(run.status, run.conclusion)}
						</span>
					</div>
					<div class="text-kai-text-muted text-sm mt-2 flex items-center gap-2">
						<span class="px-1.5 py-0.5 rounded bg-kai-bg-tertiary text-xs">{run.trigger_event}</span>
						{#if run.trigger_ref}
							<svg class="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M13 7l5 5m0 0l-5 5m5-5H6" /></svg>
							<span class="font-medium text-kai-text">{getRefDisplay(run.trigger_ref)}</span>
						{/if}
						{#if run.trigger_sha}
							<code class="font-mono text-xs bg-kai-bg-tertiary px-1.5 py-0.5 rounded">{run.trigger_sha.slice(0, 7)}</code>
						{/if}
						{#if run.created_at}
							<span class="text-xs">{formatDate(run.created_at)}</span>
						{/if}
					</div>
				</div>
				<div class="flex gap-2">
					{#if run.status === 'queued' || run.status === 'in_progress'}
						<button class="btn text-sm border-red-600/30 text-red-600 hover:bg-red-600/10" onclick={cancelRun}>Cancel</button>
					{/if}
					<button class="btn text-sm bg-kai-accent text-white hover:opacity-90" onclick={rerunWorkflow}>Re-run</button>
				</div>
			</div>
		</div>

		<!-- Jobs and Logs -->
		<div class="grid grid-cols-12 gap-4">
			<!-- Jobs Sidebar -->
			<div class="col-span-3">
				<div class="border border-kai-border rounded-lg overflow-hidden">
					<div class="px-3 py-2.5 border-b border-kai-border bg-kai-bg-secondary">
						<span class="text-sm font-medium text-kai-text-muted">Jobs</span>
					</div>
					{#each jobs as job}
						<button
							class="w-full text-left px-3 py-2.5 border-b border-kai-border last:border-b-0 transition-colors
								{selectedJob?.id === job.id ? 'bg-kai-bg-tertiary border-l-[3px] border-l-kai-accent pl-[9px]' : 'hover:bg-kai-bg-tertiary/50 border-l-[3px] border-l-transparent pl-[9px]'}"
							onclick={() => selectJob(job)}
						>
							<div class="flex items-center gap-2">
								<span class="w-5 h-5 rounded-full flex items-center justify-center text-xs font-bold
									{job.conclusion === 'failure' ? 'bg-red-600/15 text-red-600 dark:text-red-400' :
									 job.conclusion === 'success' ? 'bg-green-600/15 text-green-600 dark:text-green-400' :
									 job.status === 'in_progress' ? 'bg-blue-600/15 text-blue-600 dark:text-blue-400 animate-pulse' :
									 'bg-gray-500/15 text-gray-500'}">
									{getStatusIcon(job.status, job.conclusion)}
								</span>
								<span class="truncate flex-1 text-sm {job.conclusion === 'failure' ? 'text-red-700 dark:text-red-400 font-medium' : ''}">{job.name}</span>
								<span class="text-kai-text-muted text-xs tabular-nums">{formatDuration(job.started_at, job.completed_at)}</span>
							</div>
							{#if job.steps && job.steps.length > 0}
								<div class="ml-7 mt-1.5 space-y-0.5">
									{#each job.steps as step}
										<div class="flex items-center gap-2 text-xs
											{step.conclusion === 'failure' ? 'text-red-600 dark:text-red-400 font-medium' : 'text-kai-text-muted'}">
											<span class="w-3.5 h-3.5 flex items-center justify-center text-[10px]
												{step.conclusion === 'failure' ? 'text-red-600' :
												 step.conclusion === 'success' ? 'text-green-600' :
												 step.status === 'in_progress' ? 'text-blue-500 animate-pulse' :
												 'text-gray-400'}">
												{getStatusIcon(step.status, step.conclusion)}
											</span>
											<span class="truncate">{step.name}</span>
											{#if step.exit_code != null && step.exit_code !== 0}
												<span class="ml-auto text-[10px] font-mono text-red-500 shrink-0">exit {step.exit_code}</span>
											{/if}
										</div>
									{/each}
								</div>
							{/if}
						</button>
					{/each}
				</div>
			</div>

			<!-- Log Panel -->
			<div class="col-span-9">
				<div class="border border-kai-border rounded-lg overflow-hidden flex flex-col" style="height: 650px;">
					<!-- Log header -->
					<div class="px-4 py-2.5 border-b border-kai-border bg-kai-bg-secondary flex items-center justify-between">
						<div class="flex items-center gap-2">
							{#if selectedJob}
								<span class="text-sm font-medium">{selectedJob.name}</span>
								<span class="text-xs {getStatusColor(selectedJob.status, selectedJob.conclusion)}">{formatStatus(selectedJob.status, selectedJob.conclusion)}</span>
							{:else}
								<span class="text-sm text-kai-text-muted">Select a job</span>
							{/if}
						</div>
						<div class="flex items-center gap-2">
							{#if hasError}
								<button class="text-xs px-2 py-1 rounded bg-red-600/10 text-red-600 dark:text-red-400 hover:bg-red-600/20 transition-colors" onclick={jumpToError}>
									Jump to error
								</button>
							{/if}
							{#if selectedJob}
								<span class="text-xs text-kai-text-muted tabular-nums">{formatDuration(selectedJob.started_at, selectedJob.completed_at)}</span>
							{/if}
						</div>
					</div>

					<!-- Log content -->
					<div class="flex-1 overflow-auto bg-[#1e1e2e] text-[#cdd6f4]" bind:this={logContainer}>
						{#if logsLoading}
							<div class="p-4 text-gray-500 animate-pulse">Loading logs...</div>
						{:else if logs.length === 0}
							<div class="flex flex-col items-center justify-center h-full text-gray-500 gap-3">
								{#if selectedJob}
									{#if selectedJob.status === 'queued' || selectedJob.status === 'pending'}
										<svg class="w-8 h-8 opacity-40 animate-pulse" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5"><path stroke-linecap="round" stroke-linejoin="round" d="M12 6v6h4.5m4.5 0a9 9 0 1 1-18 0 9 9 0 0 1 18 0Z" /></svg>
										<p class="text-sm">Waiting for job to start...</p>
										<p class="text-xs opacity-60">Logs will stream here when the job begins running.</p>
									{:else if selectedJob.status === 'in_progress'}
										<svg class="w-8 h-8 opacity-40 animate-spin" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5"><path stroke-linecap="round" stroke-linejoin="round" d="M16.023 9.348h4.992v-.001M2.985 19.644v-4.992m0 0h4.992m-4.993 0 3.181 3.183a8.25 8.25 0 0 0 13.803-3.7M4.031 9.865a8.25 8.25 0 0 1 13.803-3.7l3.181 3.182m0-4.991v4.99" /></svg>
										<p class="text-sm">Job is running...</p>
										<p class="text-xs opacity-60">Logs will appear momentarily.</p>
									{:else}
										<svg class="w-8 h-8 opacity-40" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5"><path stroke-linecap="round" stroke-linejoin="round" d="M19.5 14.25v-2.625a3.375 3.375 0 0 0-3.375-3.375h-1.5A1.125 1.125 0 0 1 13.5 7.125v-1.5a3.375 3.375 0 0 0-3.375-3.375H8.25m0 12.75h7.5m-7.5 3H12M10.5 2.25H5.625c-.621 0-1.125.504-1.125 1.125v17.25c0 .621.504 1.125 1.125 1.125h12.75c.621 0 1.125-.504 1.125-1.125V11.25a9 9 0 0 0-9-9Z" /></svg>
										<p class="text-sm">No logs captured for this job.</p>
									{/if}
								{:else}
									<p class="text-sm">Select a job to view logs.</p>
								{/if}
							</div>
						{:else}
							<table class="w-full">
								<tbody>
									{#each parsedLines as line}
										{#if line.text.trim() !== ''}
											<tr class="log-line hover:bg-white/5
												{line.type === 'error' ? 'log-error bg-red-500/10' : ''}
												{line.type === 'success' ? 'log-success' : ''}
												{line.type === 'section' ? 'log-section' : ''}"
											>
												<td class="log-num select-none text-right pr-4 pl-3 text-gray-600 text-xs w-10 align-top">{line.num}</td>
												<td class="log-text pr-4 py-[1px] whitespace-pre-wrap break-all
													{line.type === 'section' ? 'text-[#89b4fa] font-bold text-[13px] py-2' : ''}
													{line.type === 'error' ? 'text-[#f38ba8]' : ''}
													{line.type === 'success' ? 'text-[#a6e3a1]' : ''}
													{line.type === 'info' ? 'text-[#94e2d5]' : ''}
													{line.type === 'muted' ? 'text-gray-600' : ''}
												">{@html ansiToHtml(line.text)}</td>
											</tr>
										{/if}
									{/each}
								</tbody>
							</table>
						{/if}
					</div>
				</div>
			</div>
		</div>
	{/if}
</div>

<style>
	.log-line {
		font-family: 'JetBrains Mono', 'Fira Code', 'SF Mono', 'Menlo', 'Consolas', monospace;
		font-size: 12px;
		line-height: 1.4;
	}
	.log-num {
		font-family: 'JetBrains Mono', 'Fira Code', 'SF Mono', 'Menlo', 'Consolas', monospace;
		font-size: 11px;
		line-height: 1.4;
		user-select: none;
		position: sticky;
		left: 0;
		background: #1e1e2e;
	}
	.tabular-nums {
		font-variant-numeric: tabular-nums;
	}
</style>
