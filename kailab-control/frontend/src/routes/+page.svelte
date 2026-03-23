<script>
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { currentUser } from '$lib/stores.js';
	import { api, loadUser } from '$lib/api.js';

	// Dashboard state
	let orgs = $state([]);
	let loading = $state(true);
	let showCreateModal = $state(false);
	let newOrgSlug = $state('');
	let newOrgName = $state('');

	// Login state
	let email = $state('');
	let magicToken = $state('');
	let stage = $state('email'); // 'email' | 'token'
	let loginError = $state('');

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

	onMount(async () => {
		const user = await loadUser();
		if (user) {
			await loadOrgs();
		}
		loading = false;
	});

	async function loadOrgs() {
		const data = await api('GET', '/api/v1/orgs');
		orgs = data.orgs || [];
	}

	async function createOrg() {
		const data = await api('POST', '/api/v1/orgs', {
			slug: newOrgSlug,
			name: newOrgName || newOrgSlug
		});
		if (data.error) {
			alert(data.error);
			return;
		}
		showCreateModal = false;
		newOrgSlug = '';
		newOrgName = '';
		await loadOrgs();
	}

	function selectOrg(slug) {
		goto(`/${slug}`);
	}

	// Login functions
	async function sendMagicLink() {
		if (!email) return;
		loginError = '';
		const data = await api('POST', '/api/v1/auth/magic-link', { email });
		if (data.error) {
			loginError = data.error;
			return;
		}
		stage = 'token';
		if (data.dev_token) {
			magicToken = data.dev_token;
		}
	}

	async function exchangeToken() {
		if (!magicToken) return;
		loginError = '';
		const data = await api('POST', '/api/v1/auth/token', { magic_token: magicToken });
		if (data.error) {
			loginError = data.error;
			return;
		}
		await loadUser();
		loading = true;
		await loadOrgs();
		loading = false;
	}

	function showEmailForm() {
		stage = 'email';
		magicToken = '';
		loginError = '';
	}
</script>

{#if $currentUser}
	<!-- ============ DASHBOARD (authenticated) ============ -->
	<div class="max-w-6xl mx-auto px-5 py-8">
		<div class="flex justify-between items-center mb-8">
			<div>
				<h2 class="text-xl font-semibold">Your Organizations</h2>
				<p class="text-sm text-kai-text-muted mt-1">Manage your organizations and repositories</p>
			</div>
			<button class="btn btn-primary" onclick={() => (showCreateModal = true)}>
				New Organization
			</button>
		</div>

		{#if loading}
			<div class="text-center py-12 text-kai-text-muted">Loading...</div>
		{:else if orgs.length === 0}
			<div class="card text-center py-16 px-8">
				<div class="mb-4 flex justify-center">
					<svg class="w-12 h-12 text-kai-text-muted" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
						<path stroke-linecap="round" stroke-linejoin="round" d="M2.25 21h19.5M3.75 21V6.75a.75.75 0 0 1 .75-.75h6a.75.75 0 0 1 .75.75V21m8.25 0V10.5a.75.75 0 0 0-.75-.75h-3a.75.75 0 0 0-.75.75V21m0-15.75h5.25a.75.75 0 0 1 .75.75v3a.75.75 0 0 1-.75.75H15" />
					</svg>
				</div>
				<p class="text-kai-text-muted mb-1 font-medium">No organizations yet</p>
				<p class="text-kai-text-muted text-sm mb-6">Create one to start managing your repositories and team.</p>
				<button class="btn btn-primary" onclick={() => (showCreateModal = true)}>
					Create your first organization
				</button>
			</div>
		{:else}
			<div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
				{#each orgs as org}
					<button
						class="card p-4 text-left w-full group hover:shadow-md hover:-translate-y-0.5 transition-all duration-150 cursor-pointer"
						onclick={() => selectOrg(org.slug)}
					>
						<div class="flex items-start gap-3">
							<div class="{avatarColor(org.name)} w-10 h-10 rounded-lg flex items-center justify-center text-white font-semibold text-sm shrink-0">
								{org.name.charAt(0).toUpperCase()}
							</div>
							<div class="min-w-0 flex-1">
								<div class="font-semibold text-kai-text truncate">{org.name}</div>
								<div class="text-sm text-kai-text-muted truncate">@{org.slug}</div>
							</div>
							<svg class="w-5 h-5 text-kai-text-muted opacity-0 group-hover:opacity-100 transition-opacity shrink-0 mt-0.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
								<path stroke-linecap="round" stroke-linejoin="round" d="M8.25 4.5l7.5 7.5-7.5 7.5" />
							</svg>
						</div>
						<div class="flex items-center gap-3 mt-3 text-xs text-kai-text-muted">
							{#if org.member_count != null}
								<span class="inline-flex items-center gap-1">
									<svg class="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
										<path stroke-linecap="round" stroke-linejoin="round" d="M15.75 6a3.75 3.75 0 1 1-7.5 0 3.75 3.75 0 0 1 7.5 0ZM4.501 20.118a7.5 7.5 0 0 1 14.998 0A17.933 17.933 0 0 1 12 21.75c-2.676 0-5.216-.584-7.499-1.632Z" />
									</svg>
									{org.member_count} {org.member_count === 1 ? 'member' : 'members'}
								</span>
							{/if}
							{#if org.role}
								<span class="inline-flex items-center px-1.5 py-0.5 rounded bg-kai-bg-tertiary text-kai-text-muted capitalize">{org.role}</span>
							{/if}
							<span class="inline-flex items-center px-1.5 py-0.5 rounded bg-kai-bg-tertiary text-kai-text-muted">{org.plan}</span>
						</div>
					</button>
				{/each}
			</div>
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
				<h3 class="text-lg font-semibold mb-4">Create Organization</h3>
				<div class="mb-4">
					<label for="org-slug" class="block mb-2 font-medium">Slug</label>
					<input
						type="text"
						id="org-slug"
						bind:value={newOrgSlug}
						class="input"
						placeholder="my-org"
						pattern="[a-z0-9._-]+"
					/>
					<small class="text-kai-text-muted">Lowercase letters, numbers, hyphens, underscores</small>
				</div>
				<div class="mb-4">
					<label for="org-name" class="block mb-2 font-medium">Name</label>
					<input
						type="text"
						id="org-name"
						bind:value={newOrgName}
						class="input"
						placeholder="My Organization"
					/>
				</div>
				<div class="flex justify-end gap-2 mt-6">
					<button class="btn" onclick={() => (showCreateModal = false)}>Cancel</button>
					<button class="btn btn-primary" onclick={createOrg}>Create</button>
				</div>
			</div>
		</div>
	{/if}

{:else if loading}
	<div class="min-h-screen flex items-center justify-center">
		<div class="text-kai-text-muted">Loading...</div>
	</div>

{:else}
	<!-- ============ LANDING PAGE (unauthenticated) ============ -->

	<!-- Nav -->
	<header class="border-b border-kai-border py-4">
		<div class="max-w-6xl mx-auto px-5 flex justify-between items-center">
			<span class="text-xl font-bold text-kai-text tracking-tight">Kai</span>
			<nav class="flex items-center gap-6">
				<a href="https://docs.kaicontext.com" target="_blank" rel="noopener noreferrer" class="text-sm text-kai-text-muted hover:text-kai-text no-underline">Docs</a>
				<a href="https://github.com/kaicontext/kai" target="_blank" rel="noopener noreferrer" class="text-sm text-kai-text-muted hover:text-kai-text no-underline">GitHub</a>
			</nav>
		</div>
	</header>

	<!-- Hero -->
	<section class="py-20 lg:py-28">
		<div class="max-w-6xl mx-auto px-5">
			<div class="grid lg:grid-cols-2 gap-16 items-center">
				<!-- Left: Copy -->
				<div>
					<p class="text-sm font-medium text-kai-accent mb-4 tracking-wide uppercase">The semantic layer for code</p>
					<h1 class="text-4xl lg:text-5xl font-bold text-kai-text leading-tight tracking-tight mb-6">
						Software development is becoming AI-native.<br/>
						<span class="text-kai-text-muted">The infrastructure isn't ready.</span>
					</h1>
					<p class="text-lg text-kai-text-muted mb-8 leading-relaxed max-w-lg">
						AI coding tools see code as text, not structure. They guess at dependencies, functions, and impact. Every guess costs tokens, time, and bugs. Kai gives them a semantic graph to reason with.
					</p>

					<!-- Git vs Kai comparison -->
					<div class="grid grid-cols-2 gap-3 mb-8">
						<div class="bg-kai-bg-secondary border border-kai-border rounded-lg p-4">
							<div class="text-xs text-kai-text-muted uppercase tracking-wide mb-3 font-medium">Git stores</div>
							<div class="space-y-2 text-sm">
								<div class="text-kai-text-muted">File diffs</div>
								<div class="text-kai-text-muted">Line changes</div>
								<div class="text-kai-text-muted">Commit hashes</div>
							</div>
						</div>
						<div class="bg-kai-bg-secondary border border-kai-accent/40 rounded-lg p-4">
							<div class="text-xs text-kai-accent uppercase tracking-wide mb-3 font-medium">Kai stores</div>
							<div class="space-y-2 text-sm">
								<div class="text-kai-text">Functions & symbols</div>
								<div class="text-kai-text">Dependencies & call graphs</div>
								<div class="text-kai-text">Behavior changes</div>
							</div>
						</div>
					</div>

					<p class="text-sm text-kai-text-muted font-medium mb-6">Git optimized for collaboration. Kai optimizes for AI reasoning.</p>

					<!-- Install command -->
					<div class="bg-kai-bg-tertiary border border-kai-border rounded-lg p-3 flex items-center gap-3">
						<code class="text-sm text-kai-text font-mono flex-1 select-all">curl -sSL https://get.kaicontext.com | sh</code>
						<button
							class="text-kai-text-muted hover:text-kai-text transition-colors shrink-0"
							title="Copy to clipboard"
							onclick={() => { navigator.clipboard.writeText('curl -sSL https://get.kaicontext.com | sh'); }}
						>
							<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24" stroke-width="2">
								<path stroke-linecap="round" stroke-linejoin="round" d="M15.666 3.888A2.25 2.25 0 0013.5 2.25h-3c-1.03 0-1.9.693-2.166 1.638m7.332 0c.055.194.084.4.084.612v0a.75.75 0 01-.75.75H9.75a.75.75 0 01-.75-.75v0c0-.212.03-.418.084-.612m7.332 0c.646.049 1.288.11 1.927.184 1.1.128 1.907 1.077 1.907 2.185V19.5a2.25 2.25 0 01-2.25 2.25H6.75A2.25 2.25 0 014.5 19.5V6.257c0-1.108.806-2.057 1.907-2.185a48.208 48.208 0 011.927-.184" />
							</svg>
						</button>
					</div>
				</div>

				<!-- Right: Login form -->
				<div class="w-full max-w-md lg:justify-self-end">
					<div class="card">
						<h2 class="text-xl font-semibold mb-1">Get started</h2>
						<p class="text-sm text-kai-text-muted mb-6">Semantic snapshots, code reviews, CI, and AI context — all in one tool.</p>

						<a href="/early-access" class="btn btn-primary w-full text-center no-underline block mb-4">Request Early Access</a>

						<div class="border-t border-kai-border pt-4">
							<p class="text-xs text-kai-text-muted mb-3 text-center">Already have access?</p>

						{#if loginError}
							<div class="alert alert-error">{loginError}</div>
						{/if}

						{#if stage === 'email'}
							<div class="mb-3">
								<input
									type="email"
									id="email"
									bind:value={email}
									class="input text-sm"
									placeholder="you@example.com"
									required
								/>
							</div>
							<button class="btn w-full text-sm" onclick={sendMagicLink}>Sign In</button>
						{:else}
							<div class="alert alert-success">Check your email for a login link!</div>
							<p class="text-center text-kai-text-muted text-sm mb-4">Or enter your token directly:</p>
							<div class="mb-4">
								<input
									type="text"
									bind:value={magicToken}
									class="input"
									placeholder="Paste token from email"
								/>
							</div>
							<button class="btn btn-primary w-full mb-4" onclick={exchangeToken}>Login</button>
							<p class="text-center">
								<button class="btn btn-sm" onclick={showEmailForm}>Try again</button>
							</p>
						{/if}
						</div>
					</div>
				</div>
			</div>
		</div>
	</section>

	<!-- Metrics bar -->
	<section class="border-y border-kai-border bg-kai-bg-secondary py-12">
		<div class="max-w-6xl mx-auto px-5">
			<div class="grid grid-cols-3 gap-8 text-center">
				<div>
					<div class="text-3xl font-bold text-kai-accent font-mono">1,247</div>
					<div class="text-sm text-kai-text-muted mt-1">Repos indexed</div>
					<div class="text-xs text-kai-text-muted mt-0.5">Across 87 organizations</div>
				</div>
				<div>
					<div class="text-3xl font-bold text-kai-warning font-mono">2.8M</div>
					<div class="text-sm text-kai-text-muted mt-1">Commits analyzed</div>
					<div class="text-xs text-kai-text-muted mt-0.5">Building semantic graphs</div>
				</div>
				<div>
					<div class="text-3xl font-bold text-kai-accent font-mono">~80%</div>
					<div class="text-sm text-kai-text-muted mt-1">CI reduction</div>
					<div class="text-xs text-kai-text-muted mt-0.5">Fewer tests, same safety</div>
				</div>
			</div>
		</div>
	</section>

	<!-- The problem -->
	<section class="py-20">
		<div class="max-w-6xl mx-auto px-5">
			<h2 class="text-2xl font-bold text-kai-text mb-2 text-center">AI coding tools are flying blind</h2>
			<p class="text-kai-text-muted text-center mb-12 max-w-2xl mx-auto">They see code as text, not structure. They guess at dependencies, functions, and impact. Kai builds the semantic graph they need to reason correctly.</p>

			<div class="grid md:grid-cols-2 gap-6">
				<div class="card group hover:border-kai-accent/40 transition-colors">
					<div class="flex items-start gap-4">
						<div class="flex-shrink-0 w-10 h-10 rounded-lg bg-kai-accent/10 flex items-center justify-center">
							<svg class="w-5 h-5 text-kai-accent" fill="none" stroke="currentColor" viewBox="0 0 24 24">
								<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M14 10l-2 1m0 0l-2-1m2 1v2.5M20 7l-2 1m2-1l-2-1m2 1v2.5M14 4l-2-1-2 1M4 7l2-1M4 7l2 1M4 7v2.5M12 21l-2-1m2 1l2-1m-2 1v-2.5M6 18l-2-1v-2.5M18 18l2-1v-2.5" />
							</svg>
						</div>
						<div>
							<h3 class="font-semibold mb-1 text-kai-text">Semantic Change Graph</h3>
							<p class="text-sm text-kai-text-muted leading-relaxed">Persistent parsing of functions, dependencies, call graphs, and behavior changes. Not file diffs — real structural understanding.</p>
						</div>
					</div>
				</div>

				<div class="card group hover:border-kai-accent/40 transition-colors">
					<div class="flex items-start gap-4">
						<div class="flex-shrink-0 w-10 h-10 rounded-lg bg-kai-purple/10 flex items-center justify-center">
							<svg class="w-5 h-5 text-kai-purple" fill="none" stroke="currentColor" viewBox="0 0 24 24">
								<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9.663 17h4.673M12 3v1m6.364 1.636l-.707.707M21 12h-1M4 12H3m3.343-5.657l-.707-.707m2.828 9.9a5 5 0 117.072 0l-.548.547A3.374 3.374 0 0014 18.469V19a2 2 0 11-4 0v-.531c0-.895-.356-1.754-.988-2.386l-.548-.547z" />
							</svg>
						</div>
						<div>
							<h3 class="font-semibold mb-1 text-kai-text">Reasoning, Not Guessing</h3>
							<p class="text-sm text-kai-text-muted leading-relaxed">AI agents query Kai's graph to understand impact before making changes. Fewer tokens, fewer bugs, better results.</p>
						</div>
					</div>
				</div>

				<div class="card group hover:border-kai-accent/40 transition-colors">
					<div class="flex items-start gap-4">
						<div class="flex-shrink-0 w-10 h-10 rounded-lg bg-kai-success/10 flex items-center justify-center">
							<svg class="w-5 h-5 text-kai-success-emphasis" fill="none" stroke="currentColor" viewBox="0 0 24 24">
								<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 10V3L4 14h7v7l9-11h-7z" />
							</svg>
						</div>
						<div>
							<h3 class="font-semibold mb-1 text-kai-text">Intelligent CI</h3>
							<p class="text-sm text-kai-text-muted leading-relaxed">Maps every code change to the exact tests that cover it. Run 20% of your test suite with 100% safety. Cut CI time by ~80%.</p>
						</div>
					</div>
				</div>

				<div class="card group hover:border-kai-accent/40 transition-colors">
					<div class="flex items-start gap-4">
						<div class="flex-shrink-0 w-10 h-10 rounded-lg bg-kai-warning/10 flex items-center justify-center">
							<svg class="w-5 h-5 text-kai-warning" fill="none" stroke="currentColor" viewBox="0 0 24 24">
								<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
							</svg>
						</div>
						<div>
							<h3 class="font-semibold mb-1 text-kai-text">Data Flywheel</h3>
							<p class="text-sm text-kai-text-muted leading-relaxed">Every commit analyzed builds richer impact models. Better AI reasoning drives more adoption. Replacing Kai means losing years of learned patterns.</p>
						</div>
					</div>
				</div>
			</div>
		</div>
	</section>

	<!-- Trajectory -->
	<section class="border-t border-kai-border bg-kai-bg-secondary py-20">
		<div class="max-w-6xl mx-auto px-5">
			<h2 class="text-2xl font-bold text-kai-text mb-2 text-center">Where we're going</h2>
			<p class="text-kai-text-muted text-center mb-12 max-w-2xl mx-auto">Each phase builds on the last. The semantic graph is the foundation.</p>

			<div class="grid md:grid-cols-3 gap-6">
				<div class="card border-kai-accent/40">
					<div class="text-xs text-kai-accent uppercase tracking-wide font-medium mb-2">Phase 1 — Shipping</div>
					<h3 class="font-semibold text-kai-text mb-2">Selective CI</h3>
					<p class="text-sm text-kai-text-muted leading-relaxed">Kai determines which tests actually need to run based on behavioral impact, not file diffs. ~80% CI time reduction for early users.</p>
				</div>
				<div class="card">
					<div class="text-xs text-kai-warning uppercase tracking-wide font-medium mb-2">Phase 2 — In Development</div>
					<h3 class="font-semibold text-kai-text mb-2">IDE Integration</h3>
					<p class="text-sm text-kai-text-muted leading-relaxed">Semantic context where developers work — precise impact analysis and change validation inside VS Code.</p>
				</div>
				<div class="card">
					<div class="text-xs text-kai-text-muted uppercase tracking-wide font-medium mb-2">Phase 3 — Building</div>
					<h3 class="font-semibold text-kai-text mb-2">Verified AI Agents</h3>
					<p class="text-sm text-kai-text-muted leading-relaxed">Agent proposes edit → Kai validates impact → agent executes with proof, not generation with hope.</p>
				</div>
			</div>
		</div>
	</section>

	<!-- Human scale vs AI scale -->
	<section class="border-t border-kai-border py-20">
		<div class="max-w-6xl mx-auto px-5">
			<h2 class="text-2xl font-bold text-kai-text mb-12 text-center">Built for the AI-native era</h2>

			<div class="grid md:grid-cols-2 gap-8">
				<div class="card">
					<h3 class="font-semibold text-kai-text-muted mb-4">Human Scale — Git Era</h3>
					<div class="space-y-3 text-sm">
						<div class="flex items-center gap-3">
							<span class="w-2 h-2 rounded-full bg-kai-text-muted shrink-0"></span>
							<span class="text-kai-text-muted">Humans write code at 1x speed</span>
						</div>
						<div class="flex items-center gap-3">
							<span class="w-2 h-2 rounded-full bg-kai-text-muted shrink-0"></span>
							<span class="text-kai-text-muted">Changes deliberate & reviewed</span>
						</div>
						<div class="flex items-center gap-3">
							<span class="w-2 h-2 rounded-full bg-kai-text-muted shrink-0"></span>
							<span class="text-kai-text-muted">Text diffs + human judgement</span>
						</div>
						<div class="flex items-center gap-3">
							<span class="w-2 h-2 rounded-full bg-kai-success-emphasis shrink-0"></span>
							<span class="text-kai-text-muted">Git works</span>
						</div>
					</div>
				</div>
				<div class="card border-kai-accent/40">
					<h3 class="font-semibold text-kai-accent mb-4">AI Scale — Emerging</h3>
					<div class="space-y-3 text-sm">
						<div class="flex items-center gap-3">
							<span class="w-2 h-2 rounded-full bg-kai-accent shrink-0"></span>
							<span class="text-kai-text">AI agents generate at 10-100x speed</span>
						</div>
						<div class="flex items-center gap-3">
							<span class="w-2 h-2 rounded-full bg-kai-accent shrink-0"></span>
							<span class="text-kai-text">Volume breaks human review</span>
						</div>
						<div class="flex items-center gap-3">
							<span class="w-2 h-2 rounded-full bg-kai-warning shrink-0"></span>
							<span class="text-kai-text">Missing: automated understanding, impact analysis</span>
						</div>
						<div class="flex items-center gap-3">
							<span class="w-2 h-2 rounded-full bg-kai-warning shrink-0"></span>
							<span class="text-kai-text">No infrastructure exists — <span class="text-kai-accent font-medium">until Kai</span></span>
						</div>
					</div>
				</div>
			</div>
		</div>
	</section>

	<!-- Language support -->
	<section class="py-20">
		<div class="max-w-6xl mx-auto px-5 text-center">
			<h2 class="text-2xl font-bold text-kai-text mb-2">Works with your stack</h2>
			<p class="text-kai-text-muted mb-10">Built-in tree-sitter grammars for the languages you use.</p>
			<div class="flex flex-wrap justify-center gap-4">
				{#each ['Go', 'TypeScript', 'JavaScript', 'Python', 'Ruby', 'Rust'] as lang}
					<span class="px-4 py-2 rounded-full border border-kai-border bg-kai-bg-secondary text-sm font-medium text-kai-text">{lang}</span>
				{/each}
			</div>
		</div>
	</section>

	<!-- Bottom CTA -->
	<section class="border-t border-kai-border bg-kai-bg-secondary py-16">
		<div class="max-w-2xl mx-auto px-5 text-center">
			<h2 class="text-2xl font-bold text-kai-text mb-3">Semantic infrastructure for code change</h2>
			<p class="text-kai-text-muted mb-6">Give your AI tools the structural understanding they need. Give your CI the intelligence it deserves.</p>
			<div class="bg-kai-bg-tertiary border border-kai-border rounded-lg p-3 flex items-center gap-3 max-w-md mx-auto mb-6">
				<code class="text-sm text-kai-text font-mono flex-1 select-all">curl -sSL https://get.kaicontext.com | sh</code>
				<button
					class="text-kai-text-muted hover:text-kai-text transition-colors shrink-0"
					title="Copy to clipboard"
					onclick={() => { navigator.clipboard.writeText('curl -sSL https://get.kaicontext.com | sh'); }}
				>
					<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24" stroke-width="2">
						<path stroke-linecap="round" stroke-linejoin="round" d="M15.666 3.888A2.25 2.25 0 0013.5 2.25h-3c-1.03 0-1.9.693-2.166 1.638m7.332 0c.055.194.084.4.084.612v0a.75.75 0 01-.75.75H9.75a.75.75 0 01-.75-.75v0c0-.212.03-.418.084-.612m7.332 0c.646.049 1.288.11 1.927.184 1.1.128 1.907 1.077 1.907 2.185V19.5a2.25 2.25 0 01-2.25 2.25H6.75A2.25 2.25 0 014.5 19.5V6.257c0-1.108.806-2.057 1.907-2.185a48.208 48.208 0 011.927-.184" />
					</svg>
				</button>
			</div>
			<div class="flex justify-center">
				<a href="#" class="btn btn-primary px-8 py-2.5 text-base no-underline" onclick={(e) => { e.preventDefault(); document.getElementById('email')?.focus(); window.scrollTo({ top: 0, behavior: 'smooth' }); }}>
					Or sign in to Kai Cloud
				</a>
			</div>
		</div>
	</section>

	<!-- Footer -->
	<footer class="border-t border-kai-border py-8">
		<div class="max-w-6xl mx-auto px-5 flex flex-col sm:flex-row justify-between items-center gap-4">
			<div class="text-sm text-kai-text-muted">&copy; 2025 Kai Layer, Inc.</div>
			<nav class="flex gap-6">
				<a href="https://docs.kaicontext.com" target="_blank" rel="noopener noreferrer" class="text-sm text-kai-text-muted hover:text-kai-text no-underline">Docs</a>
				<a href="https://github.com/kaicontext/kai" target="_blank" rel="noopener noreferrer" class="text-sm text-kai-text-muted hover:text-kai-text no-underline">GitHub</a>
			</nav>
		</div>
	</footer>
{/if}
