<script>
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/stores';
	import { currentUser, currentOrg } from '$lib/stores.js';
	import { api, loadUser } from '$lib/api.js';

	let repos = $state([]);
	let members = $state([]);
	let loading = $state(true);
	let activeTab = $state('repos');
	let showCreateModal = $state(false);
	let showAddMemberModal = $state(false);
	let newRepoName = $state('');
	let newRepoVisibility = $state('private');
	let newMemberEmail = $state('');
	let newMemberRole = $state('developer');
	let repoSearch = $state('');

	// Check if current user is admin or owner
	let isAdmin = $derived(() => {
		const user = $currentUser;
		if (!user) return false;
		const myMembership = members.find(m => m.user_id === user.id);
		return myMembership?.role === 'admin' || myMembership?.role === 'owner';
	});

	// Filtered repos based on search
	let filteredRepos = $derived(
		repoSearch
			? repos.filter(r => r.name.toLowerCase().includes(repoSearch.toLowerCase()))
			: repos
	);

	// Deterministic avatar color from org slug
	const avatarColors = [
		'bg-blue-600', 'bg-purple-600', 'bg-pink-600', 'bg-indigo-600',
		'bg-teal-600', 'bg-cyan-600', 'bg-orange-600', 'bg-rose-600',
	];

	function avatarColor(name) {
		let hash = 0;
		for (let i = 0; i < name.length; i++) {
			hash = name.charCodeAt(i) + ((hash << 5) - hash);
		}
		return avatarColors[Math.abs(hash) % avatarColors.length];
	}

	function timeAgo(dateStr) {
		const now = new Date();
		const date = new Date(dateStr);
		const seconds = Math.floor((now - date) / 1000);
		if (seconds < 60) return 'just now';
		const minutes = Math.floor(seconds / 60);
		if (minutes < 60) return `${minutes}m ago`;
		const hours = Math.floor(minutes / 60);
		if (hours < 24) return `${hours}h ago`;
		const days = Math.floor(hours / 24);
		if (days < 30) return `${days}d ago`;
		const months = Math.floor(days / 30);
		if (months < 12) return `${months}mo ago`;
		return `${Math.floor(months / 12)}y ago`;
	}

	$effect(() => {
		currentOrg.set($page.params.slug);
	});

	onMount(async () => {
		const user = await loadUser();
		if (!user) {
			goto('/login');
			return;
		}

		await loadRepos();
		await loadMembers();
	});

	let error = $state(null);

	async function loadRepos() {
		loading = true;
		error = null;
		const data = await api('GET', `/api/v1/orgs/${$page.params.slug}/repos`);
		if (data.error) {
			error = data.error;
			loading = false;
			return;
		}
		repos = data.repos || [];
		loading = false;
	}

	async function loadMembers() {
		const data = await api('GET', `/api/v1/orgs/${$page.params.slug}/members`);
		if (!data.error) {
			members = data.members || [];
		}
	}

	async function createRepo() {
		const data = await api('POST', `/api/v1/orgs/${$page.params.slug}/repos`, {
			name: newRepoName,
			visibility: newRepoVisibility
		});

		if (data.error) {
			alert(data.error);
			return;
		}

		showCreateModal = false;
		newRepoName = '';
		newRepoVisibility = 'private';
		await loadRepos();
		goto(`/${$page.params.slug}/${data.name}`);
	}

	async function addMember() {
		if (!newMemberEmail.trim()) return;

		const data = await api('POST', `/api/v1/orgs/${$page.params.slug}/members`, {
			email: newMemberEmail,
			role: newMemberRole
		});

		if (data.error) {
			alert(data.error);
			return;
		}

		showAddMemberModal = false;
		newMemberEmail = '';
		newMemberRole = 'developer';
		await loadMembers();
	}

	async function removeMember(userId) {
		if (!confirm('Are you sure you want to remove this member?')) return;

		const data = await api('DELETE', `/api/v1/orgs/${$page.params.slug}/members/${userId}`);
		if (data.error) {
			alert(data.error);
			return;
		}
		await loadMembers();
	}

	function selectRepo(name) {
		goto(`/${$page.params.slug}/${name}`);
	}

	function getRoleBadgeColor(role) {
		switch (role) {
			case 'owner': return 'bg-purple-600/10 dark:bg-purple-500/20 text-purple-700 dark:text-purple-400';
			case 'admin': return 'bg-red-600/10 dark:bg-red-500/20 text-red-700 dark:text-red-400';
			case 'maintainer': return 'bg-orange-600/10 dark:bg-orange-500/20 text-orange-700 dark:text-orange-400';
			case 'developer': return 'bg-blue-600/10 dark:bg-blue-500/20 text-blue-700 dark:text-blue-400';
			case 'reporter': return 'bg-gray-500/10 dark:bg-gray-500/20 text-gray-600 dark:text-gray-400';
			default: return 'bg-gray-500/10 dark:bg-gray-500/20 text-gray-600 dark:text-gray-400';
		}
	}
</script>

<div class="max-w-6xl mx-auto px-5 py-8">
	<!-- Header with breadcrumb and org avatar -->
	<div class="flex items-center justify-between mb-6">
		<div class="flex items-center gap-3">
			<div class="{avatarColor($page.params.slug)} w-10 h-10 rounded-lg flex items-center justify-center text-white font-semibold text-sm shrink-0">
				{$page.params.slug.charAt(0).toUpperCase()}
			</div>
			<div>
				<div class="flex items-center gap-1.5 text-sm text-kai-text-muted">
					<a href="/" class="hover:text-kai-text transition-colors no-underline">Organizations</a>
					<span>/</span>
				</div>
				<h2 class="text-xl font-semibold -mt-0.5">{$page.params.slug}</h2>
			</div>
		</div>
		{#if activeTab === 'repos'}
			<button class="btn btn-primary" onclick={() => (showCreateModal = true)}>
				<svg class="w-4 h-4 mr-1.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
					<path stroke-linecap="round" stroke-linejoin="round" d="M12 4.5v15m7.5-7.5h-15" />
				</svg>
				New Repository
			</button>
		{:else if activeTab === 'members' && isAdmin()}
			<button class="btn btn-primary" onclick={() => (showAddMemberModal = true)}>
				<svg class="w-4 h-4 mr-1.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
					<path stroke-linecap="round" stroke-linejoin="round" d="M12 4.5v15m7.5-7.5h-15" />
				</svg>
				Add Member
			</button>
		{/if}
	</div>

	<!-- Pill tabs -->
	<div class="flex items-center gap-6 mb-6">
		<div class="flex bg-kai-bg-tertiary rounded-lg p-0.5">
			<button
				class="px-3 py-1.5 text-sm font-medium rounded-md transition-colors {activeTab === 'repos' ? 'bg-kai-bg-secondary text-kai-text shadow-sm' : 'text-kai-text-muted hover:text-kai-text'}"
				onclick={() => activeTab = 'repos'}
			>
				Repositories
				{#if repos.length > 0}
					<span class="ml-1 text-kai-text-muted">({repos.length})</span>
				{/if}
			</button>
			<button
				class="px-3 py-1.5 text-sm font-medium rounded-md transition-colors {activeTab === 'members' ? 'bg-kai-bg-secondary text-kai-text shadow-sm' : 'text-kai-text-muted hover:text-kai-text'}"
				onclick={() => activeTab = 'members'}
			>
				Members
				{#if members.length > 0}
					<span class="ml-1 text-kai-text-muted">({members.length})</span>
				{/if}
			</button>
		</div>

		<!-- Search bar (repos tab only) -->
		{#if activeTab === 'repos' && repos.length > 0}
			<div class="flex-1 max-w-xs">
				<div class="relative">
					<svg class="absolute left-2.5 top-1/2 -translate-y-1/2 w-4 h-4 text-kai-text-muted" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
						<path stroke-linecap="round" stroke-linejoin="round" d="m21 21-5.197-5.197m0 0A7.5 7.5 0 1 0 5.196 5.196a7.5 7.5 0 0 0 10.607 10.607Z" />
					</svg>
					<input
						type="text"
						bind:value={repoSearch}
						class="input pl-8 !py-1.5"
						placeholder="Find a repository..."
					/>
				</div>
			</div>
		{/if}
	</div>

	{#if loading}
		<!-- Skeleton loader -->
		<div class="space-y-0">
			{#each [1, 2, 3] as _}
				<div class="flex items-center gap-4 px-4 py-4 border-b border-kai-border animate-pulse">
					<div class="w-5 h-5 bg-kai-bg-tertiary rounded"></div>
					<div class="flex-1 space-y-2">
						<div class="h-4 bg-kai-bg-tertiary rounded w-1/4"></div>
						<div class="h-3 bg-kai-bg-tertiary rounded w-1/3"></div>
					</div>
				</div>
			{/each}
		</div>
	{:else if error}
		<div class="text-center py-16 px-8">
			<div class="mb-4 flex justify-center">
				<svg class="w-12 h-12 text-kai-text-muted" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
					<path stroke-linecap="round" stroke-linejoin="round" d="M16.5 10.5V6.75a4.5 4.5 0 1 0-9 0v3.75m-.75 11.25h10.5a2.25 2.25 0 0 0 2.25-2.25v-6.75a2.25 2.25 0 0 0-2.25-2.25H6.75a2.25 2.25 0 0 0-2.25 2.25v6.75a2.25 2.25 0 0 0 2.25 2.25Z" />
				</svg>
			</div>
			<p class="font-medium text-kai-text mb-1">Organization not found or access denied</p>
			<p class="text-sm text-kai-text-muted mb-6">{error}</p>
			<a href="/" class="btn btn-primary no-underline">Go Home</a>
		</div>
	{:else if activeTab === 'repos'}
		{#if repos.length === 0}
			<div class="text-center py-16 px-8">
				<div class="mb-4 flex justify-center">
					<svg class="w-12 h-12 text-kai-text-muted" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
						<path stroke-linecap="round" stroke-linejoin="round" d="M12 6.042A8.967 8.967 0 0 0 6 3.75c-1.052 0-2.062.18-3 .512v14.25A8.987 8.987 0 0 1 6 18c2.305 0 4.408.867 6 2.292m0-14.25a8.966 8.966 0 0 1 6-2.292c1.052 0 2.062.18 3 .512v14.25A8.987 8.987 0 0 0 18 18a8.967 8.967 0 0 0-6 2.292m0-14.25v14.25" />
					</svg>
				</div>
				<p class="font-medium text-kai-text mb-1">No repositories yet</p>
				<p class="text-sm text-kai-text-muted mb-6">Create a repository to start tracking your code.</p>
				<button class="btn btn-primary" onclick={() => (showCreateModal = true)}>
					Create your first repository
				</button>
			</div>
		{:else if filteredRepos.length === 0}
			<div class="text-center py-12 text-kai-text-muted text-sm">
				No repositories matching "{repoSearch}"
			</div>
		{:else}
			<div class="border border-kai-border rounded-md overflow-hidden">
				{#each filteredRepos as repo}
					<button
						class="flex items-center w-full text-left px-4 py-3 border-b border-kai-border last:border-b-0 transition-colors duration-150 hover:bg-kai-bg group"
						onclick={() => selectRepo(repo.name)}
					>
						<!-- Visibility icon -->
						<div class="shrink-0 mr-3 text-kai-text-muted">
							{#if repo.visibility === 'private'}
								<svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
									<path stroke-linecap="round" stroke-linejoin="round" d="M16.5 10.5V6.75a4.5 4.5 0 1 0-9 0v3.75m-.75 11.25h10.5a2.25 2.25 0 0 0 2.25-2.25v-6.75a2.25 2.25 0 0 0-2.25-2.25H6.75a2.25 2.25 0 0 0-2.25 2.25v6.75a2.25 2.25 0 0 0 2.25 2.25Z" />
								</svg>
							{:else}
								<svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
									<path stroke-linecap="round" stroke-linejoin="round" d="M12 6.042A8.967 8.967 0 0 0 6 3.75c-1.052 0-2.062.18-3 .512v14.25A8.987 8.987 0 0 1 6 18c2.305 0 4.408.867 6 2.292m0-14.25a8.966 8.966 0 0 1 6-2.292c1.052 0 2.062.18 3 .512v14.25A8.987 8.987 0 0 0 18 18a8.967 8.967 0 0 0-6 2.292m0-14.25v14.25" />
								</svg>
							{/if}
						</div>

						<!-- Repo info -->
						<div class="flex-1 min-w-0">
							<span class="font-semibold text-kai-text group-hover:text-kai-accent transition-colors">{repo.name}</span>
						</div>

						<!-- Metadata -->
						<div class="flex items-center gap-3 text-xs text-kai-text-muted shrink-0">
							{#if repo.created_at}
								<span class="hidden sm:inline-flex items-center gap-1">
									<svg class="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
										<path stroke-linecap="round" stroke-linejoin="round" d="M12 6v6h4.5m4.5 0a9 9 0 1 1-18 0 9 9 0 0 1 18 0Z" />
									</svg>
									{timeAgo(repo.created_at)}
								</span>
							{/if}

							{#if repo.visibility === 'private'}
								<span class="inline-flex items-center px-2 py-0.5 rounded-full text-xs font-medium bg-amber-500/10 text-amber-700 dark:text-amber-400 border border-amber-500/20">
									Private
								</span>
							{:else}
								<span class="inline-flex items-center px-2 py-0.5 rounded-full text-xs font-medium bg-kai-bg-tertiary text-kai-text-muted">
									Public
								</span>
							{/if}

							<!-- Chevron -->
							<svg class="w-4 h-4 text-kai-text-muted opacity-0 group-hover:opacity-100 transition-all duration-150 group-hover:translate-x-0.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
								<path stroke-linecap="round" stroke-linejoin="round" d="M8.25 4.5l7.5 7.5-7.5 7.5" />
							</svg>
						</div>
					</button>
				{/each}
			</div>
		{/if}
	{:else if activeTab === 'members'}
		{#if members.length === 0}
			<div class="text-center py-16 px-8">
				<div class="mb-4 flex justify-center">
					<svg class="w-12 h-12 text-kai-text-muted" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
						<path stroke-linecap="round" stroke-linejoin="round" d="M15 19.128a9.38 9.38 0 0 0 2.625.372 9.337 9.337 0 0 0 4.121-.952 4.125 4.125 0 0 0-7.533-2.493M15 19.128v-.003c0-1.113-.285-2.16-.786-3.07M15 19.128v.106A12.318 12.318 0 0 1 8.624 21 12.3 12.3 0 0 1 2.25 19.128v-.003c0-1.113.285-2.16.786-3.07M15 19.128a9.38 9.38 0 0 1-2.625.372 9.337 9.337 0 0 1-4.121-.952M12 6.375a3.375 3.375 0 1 1-6.75 0 3.375 3.375 0 0 1 6.75 0ZM18.75 7.5a2.625 2.625 0 1 1-5.25 0 2.625 2.625 0 0 1 5.25 0Z" />
					</svg>
				</div>
				<p class="font-medium text-kai-text mb-1">No members yet</p>
				<p class="text-sm text-kai-text-muted mb-6">Invite your team to collaborate on repositories.</p>
				{#if isAdmin()}
					<button class="btn btn-primary" onclick={() => (showAddMemberModal = true)}>
						Add your first member
					</button>
				{/if}
			</div>
		{:else}
			<div class="border border-kai-border rounded-md overflow-hidden">
				{#each members as member}
					<div class="flex items-center justify-between px-4 py-3 border-b border-kai-border last:border-b-0 transition-colors duration-150 hover:bg-kai-bg">
						<div class="flex items-center gap-3">
							<img
								src={member.avatar_url}
								alt=""
								class="w-8 h-8 rounded-full bg-kai-bg-tertiary"
							/>
							<div>
								<div class="font-medium">{member.name || member.email}</div>
								{#if member.name}
									<div class="text-sm text-kai-text-muted">{member.email}</div>
								{/if}
							</div>
						</div>
						<div class="flex items-center gap-3">
							<span class="px-2 py-1 text-xs rounded-full {getRoleBadgeColor(member.role)}">{member.role}</span>
							{#if isAdmin() && member.role !== 'owner'}
								<button
									class="text-kai-text-muted hover:text-red-700 dark:hover:text-red-400 transition-colors"
									onclick={() => removeMember(member.user_id)}
									title="Remove member"
								>
									<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
										<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
									</svg>
								</button>
							{/if}
						</div>
					</div>
				{/each}
			</div>
		{/if}
	{/if}
</div>

{#if showCreateModal}
	<div
		class="fixed inset-0 bg-black/50 flex items-center justify-center z-50"
		onclick={() => (showCreateModal = false)}
		onkeydown={(e) => e.key === 'Escape' && (showCreateModal = false)}
		role="button"
		tabindex="0"
	>
		<div
			class="bg-kai-bg-secondary border border-kai-border rounded-xl p-6 max-w-md w-11/12"
			onclick={(e) => e.stopPropagation()}
			onkeydown={() => {}}
			role="dialog"
		>
			<h3 class="text-lg font-semibold mb-4">Create Repository</h3>
			<div class="mb-4">
				<label for="repo-name" class="block mb-2 font-medium">Name</label>
				<input
					type="text"
					id="repo-name"
					bind:value={newRepoName}
					class="input"
					placeholder="my-repo"
					pattern="[a-z0-9._-]+"
				/>
				<small class="text-kai-text-muted">Lowercase letters, numbers, hyphens, underscores</small>
			</div>
			<div class="mb-4">
				<label for="repo-visibility" class="block mb-2 font-medium">Visibility</label>
				<select id="repo-visibility" bind:value={newRepoVisibility} class="input">
					<option value="private">Private</option>
					<option value="public">Public</option>
				</select>
			</div>
			<div class="flex justify-end gap-2 mt-6">
				<button class="btn" onclick={() => (showCreateModal = false)}>Cancel</button>
				<button class="btn btn-primary" onclick={createRepo}>Create</button>
			</div>
		</div>
	</div>
{/if}

{#if showAddMemberModal}
	<div
		class="fixed inset-0 bg-black/50 flex items-center justify-center z-50"
		onclick={() => (showAddMemberModal = false)}
		onkeydown={(e) => e.key === 'Escape' && (showAddMemberModal = false)}
		role="button"
		tabindex="0"
	>
		<div
			class="bg-kai-bg-secondary border border-kai-border rounded-xl p-6 max-w-md w-11/12"
			onclick={(e) => e.stopPropagation()}
			onkeydown={() => {}}
			role="dialog"
		>
			<h3 class="text-lg font-semibold mb-4">Add Member</h3>
			<div class="mb-4">
				<label for="member-email" class="block mb-2 font-medium">Email</label>
				<input
					type="email"
					id="member-email"
					bind:value={newMemberEmail}
					class="input"
					placeholder="user@example.com"
				/>
			</div>
			<div class="mb-4">
				<label for="member-role" class="block mb-2 font-medium">Role</label>
				<select id="member-role" bind:value={newMemberRole} class="input">
					<option value="reporter">Reporter (read-only)</option>
					<option value="developer">Developer (push snapshots)</option>
					<option value="maintainer">Maintainer (manage repos)</option>
					<option value="admin">Admin (manage members)</option>
				</select>
			</div>
			<div class="flex justify-end gap-2 mt-6">
				<button class="btn" onclick={() => (showAddMemberModal = false)}>Cancel</button>
				<button class="btn btn-primary" onclick={addMember}>Add Member</button>
			</div>
		</div>
	</div>
{/if}
