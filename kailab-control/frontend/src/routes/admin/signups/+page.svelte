<script>
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { api, loadUser } from '$lib/api.js';

	let signups = $state([]);
	let loading = $state(true);
	let statusFilter = $state('');
	let editingId = $state(null);
	let editNotes = $state('');
	let editStatus = $state('');

	onMount(async () => {
		const user = await loadUser();
		if (!user) {
			goto('/login');
			return;
		}
		await loadSignups();
		await loadCIRequests();
	});

	async function loadSignups() {
		loading = true;
		const url = statusFilter ? `/api/v1/signups?status=${statusFilter}` : '/api/v1/signups';
		const data = await api('GET', url);
		signups = data?.signups || [];
		loading = false;
	}

	async function updateSignup(id, status, notes) {
		await api('PATCH', `/api/v1/signups/${id}`, { status, notes });
		editingId = null;
		await loadSignups();
	}

	function startEdit(signup) {
		editingId = signup.id;
		editStatus = signup.status;
		editNotes = signup.notes || '';
	}

	function copyEmail(email) {
		navigator.clipboard.writeText(email);
	}

	async function toggleCIAccess(email, currentState) {
		await api('POST', '/api/v1/admin/ci-access', { email, ci_access: !currentState });
		ciUsersByEmail[email] = { ...ciUsersByEmail[email], ci_access: !currentState };
	}

	// CI request tracking
	let ciUsersByEmail = $state({});

	async function loadCIRequests() {
		const data = await api('GET', '/api/v1/admin/ci-requests');
		if (data?.users) {
			for (const u of data.users) {
				ciUsersByEmail[u.email] = u;
			}
		}
	}

	function formatDate(d) {
		return new Date(d).toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric', hour: '2-digit', minute: '2-digit' });
	}

	function statusColor(status) {
		switch (status) {
			case 'pending_review': return 'bg-yellow-500/10 text-yellow-700 dark:text-yellow-400';
			case 'approved': return 'bg-green-500/10 text-green-700 dark:text-green-400';
			case 'rejected': return 'bg-red-500/10 text-red-700 dark:text-red-400';
			case 'contacted': return 'bg-blue-500/10 text-blue-700 dark:text-blue-400';
			default: return 'bg-gray-500/10 text-gray-600';
		}
	}
</script>

<div class="max-w-5xl mx-auto px-5 py-8">
	<div class="flex items-center justify-between mb-6">
		<h1 class="text-2xl font-semibold text-kai-text">Early Access Signups</h1>
		<div class="flex gap-2 items-center">
			<span class="text-sm text-kai-text-muted">{signups.length} total</span>
			<select bind:value={statusFilter} onchange={loadSignups} class="input text-sm">
				<option value="">All statuses</option>
				<option value="pending_review">Pending</option>
				<option value="approved">Approved</option>
				<option value="contacted">Contacted</option>
				<option value="rejected">Rejected</option>
			</select>
		</div>
	</div>

	{#if loading}
		<div class="space-y-3">
			{#each Array(5) as _}
				<div class="h-20 bg-kai-bg-tertiary rounded animate-pulse"></div>
			{/each}
		</div>
	{:else if signups.length === 0}
		<div class="text-center py-16 text-kai-text-muted">No signups yet.</div>
	{:else}
		<div class="border border-kai-border rounded-lg overflow-hidden">
			{#each signups as signup}
				<div class="border-b border-kai-border last:border-b-0 px-4 py-3 hover:bg-kai-bg-tertiary/30 transition-colors">
					<div class="flex items-start justify-between gap-4">
						<div class="flex-1 min-w-0">
							<div class="flex items-center gap-2 mb-1">
								<span class="font-medium text-kai-text">{signup.name}</span>
								<span class="px-2 py-0.5 rounded text-xs {statusColor(signup.status)}">{signup.status.replace('_', ' ')}</span>
							</div>
							<div class="flex items-center gap-3 text-sm text-kai-text-muted">
								<button onclick={() => copyEmail(signup.email)} class="hover:text-kai-text cursor-pointer flex items-center gap-1" title="Click to copy">
									{signup.email}
									<svg class="w-3 h-3" fill="none" stroke="currentColor" viewBox="0 0 24 24"><rect x="9" y="9" width="13" height="13" rx="2" ry="2" stroke-width="2"/><path d="M5 15H4a2 2 0 01-2-2V4a2 2 0 012-2h9a2 2 0 012 2v1" stroke-width="2"/></svg>
								</button>
								{#if signup.company}<span>· {signup.company}</span>{/if}
								<span>· {formatDate(signup.submitted_at)}</span>
							</div>
							{#if signup.repo_url}
								<div class="text-xs text-kai-text-muted mt-1">
									<a href={signup.repo_url} target="_blank" class="hover:text-kai-accent">{signup.repo_url}</a>
								</div>
							{/if}
							{#if signup.ai_usage}
								<div class="text-xs text-kai-text-muted mt-1 italic">"{signup.ai_usage}"</div>
							{/if}
							{#if signup.notes}
								<div class="text-xs mt-1 text-kai-text-muted bg-kai-bg-tertiary rounded px-2 py-1">Notes: {signup.notes}</div>
							{/if}
						</div>
						<div class="shrink-0">
							{#if editingId === signup.id}
								<div class="flex flex-col gap-2 items-end">
									<select bind:value={editStatus} class="input text-xs w-36">
										<option value="pending_review">Pending</option>
										<option value="approved">Approved</option>
										<option value="contacted">Contacted</option>
										<option value="rejected">Rejected</option>
									</select>
									<input type="text" bind:value={editNotes} class="input text-xs w-36" placeholder="Notes..." />
									<div class="flex gap-1">
										<button class="btn text-xs" onclick={() => editingId = null}>Cancel</button>
										<button class="btn btn-primary text-xs" onclick={() => updateSignup(signup.id, editStatus, editNotes)}>Save</button>
									</div>
								</div>
							{:else}
								<div class="flex gap-1 items-center">
								{#if signup.status === 'approved'}
									{@const ciUser = ciUsersByEmail[signup.email]}
									<button
										class="btn text-xs {ciUser?.ci_access ? 'btn-primary' : ciUser?.ci_requested ? 'border-yellow-500 text-yellow-600' : ''}"
										onclick={() => toggleCIAccess(signup.email, ciUser?.ci_access || false)}
										title={ciUser?.ci_access ? 'CI access enabled (click to revoke)' : ciUser?.ci_requested ? 'User requested CI access (click to grant)' : 'Grant CI access'}
									>
										{#if ciUser?.ci_access}
											CI On
										{:else if ciUser?.ci_requested}
											CI Requested
										{:else}
											CI Off
										{/if}
									</button>
								{/if}
								<button class="btn text-xs" onclick={() => startEdit(signup)}>Edit</button>
							</div>
							{/if}
						</div>
					</div>
				</div>
			{/each}
		</div>
	{/if}
</div>
