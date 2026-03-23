<script>
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/stores';
	import { currentUser } from '$lib/stores.js';
	import { api, loadUser } from '$lib/api.js';
	import RepoNav from '$lib/components/RepoNav.svelte';

	let entries = $state([]);
	let commits = $state([]);
	let loading = $state(true);
	let error = $state('');

	onMount(async () => {
		const user = await loadUser();
		if (!user) {
			goto('/login');
			return;
		}
		await loadEntries();
	});

	async function loadEntries() {
		loading = true;
		error = '';
		const { slug, repo } = $page.params;

		try {
			const data = await api('GET', `/${slug}/${repo}/v1/log/entries?limit=50`);
			if (data.error) {
				error = data.error;
				entries = [];
			} else {
				entries = data.entries || [];
				commits = buildCommits(entries);
				await loadChangesetDetails();
			}
		} catch (e) {
			error = 'Failed to load history';
			entries = [];
		}

		loading = false;
	}

	// Group raw log entries into meaningful "commits"
	// A push typically produces: snap.X update + cs.X update at the same time
	// We want to show one row per logical change, preferring changeset info
	function buildCommits(entries) {
		// Group entries that happened within 2 seconds of each other by the same actor
		const groups = [];
		let current = null;

		for (const entry of entries) {
			if (current && current.actor === entry.actor && Math.abs(current.time - entry.time) < 2000) {
				current.entries.push(entry);
			} else {
				current = {
					actor: entry.actor,
					time: entry.time,
					entries: [entry],
				};
				groups.push(current);
			}
		}

		// For each group, extract the most useful info
		return groups.map((group) => {
			const csEntry = group.entries.find((e) => e.ref && e.ref.startsWith('cs.'));
			const snapEntry = group.entries.find((e) => e.ref && e.ref.startsWith('snap.'));
			const reviewEntry = group.entries.find((e) => e.ref && e.ref.startsWith('review.'));

			// Prefer changeset as the primary entry
			const primary = csEntry || snapEntry || group.entries[0];

			return {
				id: primary.id,
				time: group.time,
				actor: group.actor,
				// Changeset info
				changesetDigest: csEntry ? hexEncode(csEntry.new) : null,
				// Snapshot info
				snapshotDigest: snapEntry ? hexEncode(snapEntry.new) : null,
				snapshotOld: snapEntry ? hexEncode(snapEntry.old) : null,
				snapshotRef: snapEntry ? snapEntry.ref : null,
				// Review info
				reviewId: reviewEntry ? formatRefName(reviewEntry.ref) : null,
				// What refs were updated
				refs: group.entries.map((e) => e.ref).filter(Boolean),
				// Changeset details (filled in later)
				changeset: null,
			};
		});
	}

	async function loadChangesetDetails() {
		const { slug, repo } = $page.params;

		// Only fetch first 20 changesets to avoid hammering the API
		const toFetch = commits.filter((c) => c.changesetDigest).slice(0, 20);

		// Fetch in batches of 5 for concurrency control
		for (let i = 0; i < toFetch.length; i += 5) {
			const batch = toFetch.slice(i, i + 5);
			await Promise.all(batch.map(async (commit) => {
				try {
					const data = await api('GET', `/${slug}/${repo}/v1/changesets/${commit.changesetDigest}`);
					if (!data.error) {
						commit.changeset = data;
					}
				} catch (e) {
					// ignore
				}
			}));
		}
		// Trigger reactivity
		commits = [...commits];
	}

	function formatDate(timestamp) {
		if (!timestamp) return '';
		return new Date(timestamp).toLocaleDateString('en-US', {
			month: 'short',
			day: 'numeric',
			year: 'numeric',
			hour: '2-digit',
			minute: '2-digit',
		});
	}

	function formatRelativeTime(timestamp) {
		if (!timestamp) return '';
		const now = Date.now();
		const diff = now - timestamp;
		const seconds = Math.floor(diff / 1000);
		const minutes = Math.floor(seconds / 60);
		const hours = Math.floor(minutes / 60);
		const days = Math.floor(hours / 24);

		if (days > 7) {
			return formatDate(timestamp);
		} else if (days > 0) {
			return `${days}d ago`;
		} else if (hours > 0) {
			return `${hours}h ago`;
		} else if (minutes > 0) {
			return `${minutes}m ago`;
		} else {
			return 'just now';
		}
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

	function formatRefName(refName) {
		if (!refName) return '';
		if (refName.startsWith('snap.')) return refName.replace('snap.', '');
		if (refName.startsWith('cs.')) return refName.replace('cs.', '');
		if (refName.startsWith('review.')) return refName.replace('review.', '');
		if (refName.startsWith('ws.')) return refName.replace('ws.', '');
		return refName;
	}

	function getChangeCount(commit) {
		if (!commit.changeset || !commit.changeset.files) return null;
		return commit.changeset.files.length;
	}

	function getFileSummary(commit) {
		if (!commit.changeset || !commit.changeset.files) return null;
		const files = commit.changeset.files;
		const added = files.filter((f) => f.action === 'added').length;
		const modified = files.filter((f) => f.action === 'modified').length;
		const deleted = files.filter((f) => f.action === 'deleted').length;
		const parts = [];
		if (added) parts.push(`${added} added`);
		if (modified) parts.push(`${modified} modified`);
		if (deleted) parts.push(`${deleted} deleted`);
		return parts.join(', ');
	}

	function viewDiff(commit) {
		if (commit.changesetDigest) {
			goto(`/${$page.params.slug}/${$page.params.repo}/changes/${shortHex(commit.changesetDigest)}`);
		} else if (commit.snapshotDigest) {
			goto(`/${$page.params.slug}/${$page.params.repo}?snap=${shortHex(commit.snapshotDigest)}`);
		}
	}
</script>

<div class="max-w-6xl mx-auto px-5 py-8">
	<RepoNav active="history" />

	{#if loading}
		<div class="text-center py-12 text-kai-text-muted">Loading...</div>
	{:else if error}
		<div class="card text-center py-12">
			<p class="text-red-700 dark:text-red-400 mb-4">{error}</p>
			<button class="btn" onclick={loadEntries}>Retry</button>
		</div>
	{:else if commits.length === 0}
		<div class="card text-center py-12">
			<p class="text-kai-text-muted mb-4">No history yet</p>
			<p class="text-kai-text-muted text-sm">
				Push changes with <code class="bg-kai-bg-tertiary px-2 py-1 rounded">kai push</code>
			</p>
		</div>
	{:else}
		<div class="card p-0">
			{#each commits as commit, i}
				{@const fileCount = getChangeCount(commit)}
				{@const fileSummary = getFileSummary(commit)}
				<div
					class="list-item {i < commits.length - 1 ? 'border-b border-kai-border' : ''} cursor-pointer hover:bg-kai-bg-tertiary/50 transition-colors"
					onclick={() => viewDiff(commit)}
					onkeydown={(e) => e.key === 'Enter' && viewDiff(commit)}
					role="button"
					tabindex="0"
				>
					<div class="flex-1 min-w-0">
						<!-- Message / intent -->
						<div class="text-sm font-medium">
							{#if commit.changeset?.intent}
								{commit.changeset.intent}
							{:else if commit.reviewId}
								Review: {commit.reviewId}
							{:else if commit.snapshotRef}
								Updated {commit.snapshotRef}
							{:else}
								Push {shortHex(hexEncode(commit.id))}
							{/if}
						</div>

						<!-- Meta line -->
						<div class="text-kai-text-muted text-xs mt-1.5 flex items-center gap-3 flex-wrap">
							<span>{commit.actor || 'unknown'}</span>
							<span title={formatDate(commit.time)}>
								{formatRelativeTime(commit.time)}
							</span>
							{#if commit.snapshotDigest}
								<span class="font-mono text-kai-text-muted/70" title="Snapshot: {commit.snapshotDigest}">
									{shortHex(commit.snapshotDigest)}
								</span>
							{/if}
						</div>

						<!-- File changes summary -->
						{#if fileSummary}
							<div class="text-xs mt-1.5 text-kai-text-muted">
								{fileSummary}
							</div>
						{/if}
					</div>

					<!-- Right side -->
					<div class="flex items-center gap-3 shrink-0">
						{#if fileCount !== null}
							<span class="text-xs text-kai-text-muted font-mono">
								{fileCount} file{fileCount !== 1 ? 's' : ''}
							</span>
						{/if}
						{#if commit.changesetDigest}
							<svg class="w-4 h-4 text-kai-text-muted" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
								<path stroke-linecap="round" stroke-linejoin="round" d="M8.25 4.5l7.5 7.5-7.5 7.5" />
							</svg>
						{/if}
					</div>
				</div>
			{/each}
		</div>
	{/if}
</div>
