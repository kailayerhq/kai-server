<script>
	import { page } from '$app/stores';

	let { active = '' } = $props();

	let slug = $derived($page.params.slug);
	let repo = $derived($page.params.repo);

	const tabs = [
		{ id: 'readme', label: 'README', href: () => `/${slug}/${repo}`, icon: 'book' },
		{ id: 'files', label: 'Files', href: () => `/${slug}/${repo}/snap.latest`, icon: 'folder' },
		{ id: 'reviews', label: 'Reviews', href: () => `/${slug}/${repo}/reviews`, icon: 'clipboard' },
		{ id: 'history', label: 'History', href: () => `/${slug}/${repo}/commits`, icon: 'clock' },
		{ id: 'workspaces', label: 'Workspaces', href: () => `/${slug}/${repo}/workspaces`, icon: 'branches' },
		{ id: 'ci', label: 'CI', href: () => `/${slug}/${repo}/workflows`, icon: 'check-circle' },
	];
</script>

<!-- Org breadcrumb header -->
<div class="flex items-center gap-2 mb-4">
	<div class="flex items-center gap-1.5 text-sm">
		<a href="/" class="text-kai-text-muted hover:text-kai-text no-underline transition-colors">Organizations</a>
		<span class="text-kai-text-muted">/</span>
		<a href="/{slug}" class="text-kai-text-muted hover:text-kai-text no-underline transition-colors">{slug}</a>
		<span class="text-kai-text-muted">/</span>
		<span class="text-kai-text font-semibold">{repo}</span>
	</div>
</div>

<div class="flex items-center justify-between border-b border-kai-border mb-6">
	<nav class="flex -mb-px">
		{#each tabs as tab}
			<a href={tab.href()} class="repo-tab {active === tab.id ? 'repo-tab-active' : ''} no-underline">
				{#if tab.icon === 'book'}
					<svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
						<path stroke-linecap="round" stroke-linejoin="round" d="M12 6.042A8.967 8.967 0 0 0 6 3.75c-1.052 0-2.062.18-3 .512v14.25A8.987 8.987 0 0 1 6 18c2.305 0 4.408.867 6 2.292m0-14.25a8.966 8.966 0 0 1 6-2.292c1.052 0 2.062.18 3 .512v14.25A8.987 8.987 0 0 0 18 18a8.967 8.967 0 0 0-6 2.292m0-14.25v14.25" />
					</svg>
				{:else if tab.icon === 'folder'}
					<svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
						<path stroke-linecap="round" stroke-linejoin="round" d="M2.25 12.75V12A2.25 2.25 0 0 1 4.5 9.75h15A2.25 2.25 0 0 1 21.75 12v.75m-8.69-6.44-2.12-2.12a1.5 1.5 0 0 0-1.061-.44H4.5A2.25 2.25 0 0 0 2.25 6v12a2.25 2.25 0 0 0 2.25 2.25h15A2.25 2.25 0 0 0 21.75 18V9a2.25 2.25 0 0 0-2.25-2.25h-5.379a1.5 1.5 0 0 1-1.06-.44Z" />
					</svg>
				{:else if tab.icon === 'clipboard'}
					<svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
						<path stroke-linecap="round" stroke-linejoin="round" d="M9 5H7a2 2 0 0 0-2 2v12a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V7a2 2 0 0 0-2-2h-2M9 5a2 2 0 0 0 2 2h2a2 2 0 0 0 2-2M9 5a2 2 0 0 1 2-2h2a2 2 0 0 1 2 2" />
					</svg>
				{:else if tab.icon === 'clock'}
					<svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
						<path stroke-linecap="round" stroke-linejoin="round" d="M12 6v6h4.5m4.5 0a9 9 0 1 1-18 0 9 9 0 0 1 18 0Z" />
					</svg>
				{:else if tab.icon === 'branches'}
					<svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
						<path stroke-linecap="round" stroke-linejoin="round" d="M6 3v12m0 0a3 3 0 1 0 3 3m-3-3a3 3 0 0 1 3 3m0 0h6m0 0a3 3 0 1 0 3-3m-3 3a3 3 0 0 1 3-3m0 0V9m0 0a3 3 0 1 0-3-3m3 3a3 3 0 0 1-3-3m0 0H9" />
					</svg>
				{:else if tab.icon === 'check-circle'}
					<svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
						<path stroke-linecap="round" stroke-linejoin="round" d="M9 12.75 11.25 15 15 9.75M21 12a9 9 0 1 1-18 0 9 9 0 0 1 18 0Z" />
					</svg>
				{/if}
				{tab.label}
			</a>
		{/each}
	</nav>
	<a href="/{slug}/{repo}/settings" class="repo-tab no-underline" title="Settings">
		<svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
			<path stroke-linecap="round" stroke-linejoin="round" d="M9.594 3.94c.09-.542.56-.94 1.11-.94h2.593c.55 0 1.02.398 1.11.94l.213 1.281c.063.374.313.686.645.87.074.04.147.083.22.127.325.196.72.257 1.075.124l1.217-.456a1.125 1.125 0 0 1 1.37.49l1.296 2.247a1.125 1.125 0 0 1-.26 1.431l-1.003.827c-.293.241-.438.613-.43.992a7.723 7.723 0 0 1 0 .255c-.008.378.137.75.43.991l1.004.827c.424.35.534.955.26 1.43l-1.298 2.247a1.125 1.125 0 0 1-1.369.491l-1.217-.456c-.355-.133-.75-.072-1.076.124a6.47 6.47 0 0 1-.22.128c-.331.183-.581.495-.644.869l-.213 1.281c-.09.543-.56.94-1.11.94h-2.594c-.55 0-1.019-.398-1.11-.94l-.213-1.281c-.062-.374-.312-.686-.644-.87a6.52 6.52 0 0 1-.22-.127c-.325-.196-.72-.257-1.076-.124l-1.217.456a1.125 1.125 0 0 1-1.369-.49l-1.297-2.247a1.125 1.125 0 0 1 .26-1.431l1.004-.827c.292-.24.437-.613.43-.991a6.932 6.932 0 0 1 0-.255c.007-.38-.138-.751-.43-.992l-1.004-.827a1.125 1.125 0 0 1-.26-1.43l1.297-2.247a1.125 1.125 0 0 1 1.37-.491l1.216.456c.356.133.751.072 1.076-.124.072-.044.146-.086.22-.128.332-.183.582-.495.644-.869l.214-1.28Z" />
			<path stroke-linecap="round" stroke-linejoin="round" d="M15 12a3 3 0 1 1-6 0 3 3 0 0 1 6 0Z" />
		</svg>
	</a>
</div>

<style>
	.repo-tab {
		display: inline-flex;
		align-items: center;
		gap: 0.375rem;
		padding: 0.5rem 0.75rem;
		font-size: 0.875rem;
		font-weight: 500;
		color: rgb(var(--kai-text-muted));
		border-bottom: 2px solid transparent;
		transition: color 0.15s, border-color 0.15s;
		white-space: nowrap;
	}

	.repo-tab:hover {
		color: rgb(var(--kai-text));
	}

	.repo-tab-active {
		color: rgb(var(--kai-text));
		border-bottom-color: rgb(var(--kai-accent));
	}
</style>
