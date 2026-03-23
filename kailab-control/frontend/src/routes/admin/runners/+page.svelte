<script>
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { api, loadUser } from '$lib/api.js';

	let ciRequests = $state([]);
	let loading = $state(true);
	let actionInFlight = $state(null);

	onMount(async () => {
		const user = await loadUser();
		if (!user) {
			goto('/login');
			return;
		}
		await loadCIRequests();
	});

	async function loadCIRequests() {
		loading = true;
		const data = await api('GET', '/api/v1/admin/ci-requests');
		ciRequests = data?.users || [];
		loading = false;
	}

	async function setCIAccess(email, grant) {
		actionInFlight = email;
		await api('POST', '/api/v1/admin/ci-access', { email, ci_access: grant });
		await loadCIRequests();
		actionInFlight = null;
	}

	function accessStatusColor(user) {
		if (user.ci_access) return 'bg-green-500/10 text-green-700 dark:text-green-400';
		if (user.ci_requested) return 'bg-yellow-500/10 text-yellow-700 dark:text-yellow-400';
		return 'bg-gray-500/10 text-gray-600';
	}

	function accessStatusLabel(user) {
		if (user.ci_access) return 'active';
		if (user.ci_requested) return 'requested';
		return 'none';
	}

	let pendingRequests = $derived(ciRequests.filter(u => u.ci_requested && !u.ci_access));
	let activeUsers = $derived(ciRequests.filter(u => u.ci_access));
</script>

<div class="max-w-5xl mx-auto px-5 py-8">
	<div class="flex items-center justify-between mb-6">
		<div>
			<h1 class="text-2xl font-semibold text-kai-text">CI Admin</h1>
			<p class="text-sm text-kai-text-muted mt-1">Manage CI access requests and runner status</p>
		</div>
		<a href="/admin/signups" class="btn text-sm no-underline">Signups</a>
	</div>

	{#if loading}
		<div class="space-y-3">
			{#each Array(4) as _}
				<div class="h-16 bg-kai-bg-tertiary rounded animate-pulse"></div>
			{/each}
		</div>
	{:else}
		<!-- Pending Requests -->
		<section class="mb-8">
			<div class="flex items-center gap-3 mb-4">
				<h2 class="text-lg font-medium text-kai-text">Pending Requests</h2>
				{#if pendingRequests.length > 0}
					<span class="px-2 py-0.5 rounded text-xs bg-yellow-500/10 text-yellow-700 dark:text-yellow-400">
						{pendingRequests.length}
					</span>
				{/if}
			</div>

			{#if pendingRequests.length === 0}
				<div class="text-center py-8 text-kai-text-muted border border-kai-border rounded-lg text-sm">
					No pending CI access requests.
				</div>
			{:else}
				<div class="border border-kai-border rounded-lg overflow-hidden">
					{#each pendingRequests as user}
						<div class="border-b border-kai-border last:border-b-0 px-4 py-3 hover:bg-kai-bg-tertiary/30 transition-colors">
							<div class="flex items-center justify-between gap-4">
								<div class="flex items-center gap-2">
									<span class="font-medium text-kai-text">{user.email}</span>
									{#if user.name}
										<span class="text-kai-text-muted text-sm">({user.name})</span>
									{/if}
									<span class="px-2 py-0.5 rounded text-xs {accessStatusColor(user)}">{accessStatusLabel(user)}</span>
								</div>
								<button
									class="btn btn-primary text-xs"
									disabled={actionInFlight === user.email}
									onclick={() => setCIAccess(user.email, true)}
								>
									{actionInFlight === user.email ? 'Approving...' : 'Approve'}
								</button>
							</div>
						</div>
					{/each}
				</div>
			{/if}
		</section>

		<!-- Active CI Users -->
		{#if activeUsers.length > 0}
			<section class="mb-8">
				<h2 class="text-lg font-medium text-kai-text mb-4">Active CI Access ({activeUsers.length})</h2>
				<div class="border border-kai-border rounded-lg overflow-hidden">
					{#each activeUsers as user}
						<div class="border-b border-kai-border last:border-b-0 px-4 py-3 hover:bg-kai-bg-tertiary/30 transition-colors">
							<div class="flex items-center justify-between gap-4">
								<div class="flex items-center gap-2">
									<span class="font-medium text-kai-text">{user.email}</span>
									{#if user.name}
										<span class="text-kai-text-muted text-sm">({user.name})</span>
									{/if}
									<span class="px-2 py-0.5 rounded text-xs {accessStatusColor(user)}">{accessStatusLabel(user)}</span>
								</div>
								<button
									class="btn text-xs"
									disabled={actionInFlight === user.email}
									onclick={() => setCIAccess(user.email, false)}
								>
									{actionInFlight === user.email ? 'Revoking...' : 'Revoke'}
								</button>
							</div>
						</div>
					{/each}
				</div>
			</section>
		{/if}

		<!-- Runners -->
		<section>
			<h2 class="text-lg font-medium text-kai-text mb-4">Runners</h2>
			<div class="border border-kai-border rounded-lg p-6 text-center text-kai-text-muted">
				<svg class="w-8 h-8 mx-auto mb-3 opacity-50" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
					<path stroke-linecap="round" stroke-linejoin="round" d="M5.25 14.25h13.5m-13.5 0a3 3 0 0 1-3-3m3 3a3 3 0 1 0 0 6h13.5a3 3 0 1 0 0-6m-16.5-3a3 3 0 0 1 3-3h13.5a3 3 0 0 1 3 3m-19.5 0a4.5 4.5 0 0 1 .9-2.7L5.737 5.1a3.375 3.375 0 0 1 2.7-1.35h7.126c1.062 0 2.062.5 2.7 1.35l2.587 3.45a4.5 4.5 0 0 1 .9 2.7m0 0a3 3 0 0 1-3 3m0 3h.008v.008h-.008v-.008Zm0-6h.008v.008h-.008v-.008Zm-3 6h.008v.008h-.008v-.008Zm0-6h.008v.008h-.008v-.008Z" />
				</svg>
				<p class="text-sm">Runner fleet dashboard coming soon.</p>
				<p class="text-xs mt-1">Runners are managed via <code>kubectl</code> in the <code>kailab-ci</code> namespace.</p>
			</div>
		</section>
	{/if}
</div>
