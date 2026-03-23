<script>
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/stores';
	import { currentUser, currentOrg, currentRepo } from '$lib/stores.js';
	import { api, loadUser } from '$lib/api.js';
	import { marked } from 'marked';
	import RepoNav from '$lib/components/RepoNav.svelte';

	// Configure marked for GitHub-style markdown
	marked.setOptions({
		gfm: true,
		breaks: true
	});

	/**
	 * Sanitize HTML by removing potentially dangerous tags and attributes
	 */
	function sanitizeHtml(html) {
		if (!html) return '';
		return html
			.replace(/<script\b[^<]*(?:(?!<\/script>)<[^<]*)*<\/script>/gi, '')
			.replace(/<iframe\b[^<]*(?:(?!<\/iframe>)<[^<]*)*<\/iframe>/gi, '')
			.replace(/<object\b[^<]*(?:(?!<\/object>)<[^<]*)*<\/object>/gi, '')
			.replace(/<embed\b[^>]*>/gi, '')
			.replace(/\bon\w+\s*=/gi, 'data-removed=')
			.replace(/javascript:/gi, 'removed:');
	}

	function base64ToUtf8(b64) {
		const binaryStr = atob(b64);
		const bytes = new Uint8Array(binaryStr.length);
		for (let i = 0; i < binaryStr.length; i++) {
			bytes[i] = binaryStr.charCodeAt(i);
		}
		return new TextDecoder().decode(bytes);
	}

	function safeMarkdown(content) {
		const renderer = new marked.Renderer();
		const defaultImageRenderer = renderer.image.bind(renderer);
		renderer.image = function({ href, title, text }) {
			if (href && !href.startsWith('http://') && !href.startsWith('https://') && !href.startsWith('data:')) {
				const cleanPath = href.replace(/^\.\//, '');
				const digest = fileDigestMap[cleanPath];
				if (digest) {
					href = `/${slug}/${repo}/v1/raw/${digest}`;
				}
			}
			// Wrap images with error handling
			const alt = text ? ` alt="${text}"` : ' alt=""';
			const titleAttr = title ? ` title="${title}"` : '';
			return `<img src="${href}"${alt}${titleAttr} loading="lazy" onerror="this.parentElement.replaceChild(Object.assign(document.createElement('div'),{className:'img-fallback',innerHTML:'<svg width=\\'24\\' height=\\'24\\' viewBox=\\'0 0 24 24\\' fill=\\'none\\' stroke=\\'currentColor\\' stroke-width=\\'1.5\\'><path stroke-linecap=\\'round\\' stroke-linejoin=\\'round\\' d=\\'m2.25 15.75 5.159-5.159a2.25 2.25 0 0 1 3.182 0l5.159 5.159m-1.5-1.5 1.409-1.409a2.25 2.25 0 0 1 3.182 0l2.909 2.909M3.75 21h16.5A2.25 2.25 0 0 0 22.5 18.75V5.25A2.25 2.25 0 0 0 20.25 3H3.75A2.25 2.25 0 0 0 1.5 5.25v13.5A2.25 2.25 0 0 0 3.75 21Z\\'/></svg>' + (this.alt ? '<span>' + this.alt + '</span>' : '')}),this)" />`;
		};
		renderer.code = function({ text, lang }) {
			const escaped = text.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
			const langClass = lang ? ` class="language-${lang}"` : '';
			const langLabel = lang ? `<span class="code-lang-label">${lang}</span>` : '';
			return `<div class="code-block-wrapper">${langLabel}<button class="copy-code-btn" title="Copy code"><svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="9" y="9" width="13" height="13" rx="2" ry="2"/><path d="M5 15H4a2 2 0 01-2-2V4a2 2 0 012-2h9a2 2 0 012 2v1"/></svg></button><pre><code${langClass}>${escaped}</code></pre></div>`;
		};
		// Suppress the first H1 if it matches the repo name to avoid double title
		let firstH1Suppressed = false;
		const defaultHeading = renderer.heading.bind(renderer);
		renderer.heading = function({ tokens, depth }) {
			const text = this.parser.parseInline(tokens);
			if (depth === 1 && !firstH1Suppressed) {
				const headingText = text.replace(/<[^>]+>/g, '').trim().toLowerCase();
				if (headingText === repo.toLowerCase()) {
					firstH1Suppressed = true;
					return '';
				}
			}
			const id = text.replace(/<[^>]+>/g, '').trim().toLowerCase().replace(/[^\w]+/g, '-');
			return `<h${depth} id="${id}">${text}</h${depth}>`;
		};
		let html = sanitizeHtml(marked(content, { renderer }));
		// Rewrite relative src in raw HTML <img> tags (not caught by marked's image renderer)
		html = html.replace(/<img\s([^>]*?)src="([^"]+)"([^>]*?)>/g, (match, before, src, after) => {
			if (src.startsWith('http://') || src.startsWith('https://') || src.startsWith('data:') || src.startsWith('/')) {
				return match;
			}
			const cleanPath = src.replace(/^\.\//, '');
			const digest = fileDigestMap[cleanPath];
			if (digest) {
				return `<img ${before}src="/${slug}/${repo}/v1/raw/${digest}"${after}>`;
			}
			return match;
		});
		return html;
	}

	function handleMarkdownClick(e) {
		// Handle copy-code button
		const btn = e.target.closest('.copy-code-btn');
		if (btn) {
			const wrapper = btn.closest('.code-block-wrapper');
			const code = wrapper?.querySelector('code');
			if (!code) return;
			navigator.clipboard.writeText(code.textContent).then(() => {
				btn.classList.add('copied');
				btn.innerHTML = '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="20 6 9 17 4 12"/></svg>';
				setTimeout(() => {
					btn.classList.remove('copied');
					btn.innerHTML = '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="9" y="9" width="13" height="13" rx="2" ry="2"/><path d="M5 15H4a2 2 0 01-2-2V4a2 2 0 012-2h9a2 2 0 012 2v1"/></svg>';
				}, 2000);
			});
			return;
		}

		// Handle link clicks — SPA navigate for internal links
		const link = e.target.closest('a');
		if (!link) return;
		const href = link.getAttribute('href');
		if (!href) return;

		// External links — let browser handle
		if (href.startsWith('http://') || href.startsWith('https://')) return;

		// Anchor links (e.g. #section)
		if (href.startsWith('#')) return;

		e.preventDefault();
		const { slug, repo } = $page.params;

		// Relative path — resolve against current repo file tree
		const resolved = href.startsWith('/') ? href : `/${slug}/${repo}/${href}`;
		goto(resolved);
	}

	function isReadme(path) {
		const filename = path.split('/').pop()?.toLowerCase() || '';
		return filename === 'readme.md' || filename === 'readme' || filename === 'readme.txt' || filename === 'readme.markdown';
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

	let { slug, repo } = $page.params;

	// Sidebar: detect special files
	let hasLicense = $derived(allFiles.some(f => f.path.toLowerCase().startsWith('license')));
	let hasContributing = $derived(allFiles.some(f => f.path.toLowerCase().startsWith('contributing')));
	let hasSecurity = $derived(allFiles.some(f => f.path.toLowerCase().startsWith('security')));
	let hasChangelog = $derived(allFiles.some(f => f.path.toLowerCase().startsWith('changelog')));

	// Detect languages from file extensions
	let languages = $derived(() => {
		const langMap = {};
		for (const f of allFiles) {
			const lang = f.lang || '';
			if (lang && lang !== 'unknown') {
				langMap[lang] = (langMap[lang] || 0) + 1;
			}
		}
		return Object.entries(langMap).sort((a, b) => b[1] - a[1]).slice(0, 8);
	});

	let loading = $state(true);
	let error = $state(null);
	let readmeContent = $state('');
	let readmeFile = $state(null);
	let repoInfo = $state(null);
	let latestSnapshot = $state(null);
	let fileCount = $state(0);
	let fileDigestMap = $state({});
	let allFiles = $state([]);
	let snapshotCount = $state(0);
	let changesetCount = $state(0);
	let reviewCount = $state(0);


	onMount(async () => {
		await loadUser();
		await loadRepoData();
	});

	async function loadRepoData() {
		loading = true;
		error = null;

		try {
			// Parallel fetch: org, repo, refs, reviews all at once
			const [orgData, repoData, allRefsData, reviewsData] = await Promise.all([
				api('GET', `/api/v1/orgs/${slug}`),
				api('GET', `/api/v1/orgs/${slug}/repos/${repo}`),
				api('GET', `/${slug}/${repo}/v1/refs`).catch(() => ({ refs: [] })),
				api('GET', `/${slug}/${repo}/v1/reviews`).catch(() => ({ reviews: [] })),
			]);

			if (!orgData || orgData.error) {
				error = 'Organization not found';
				loading = false;
				return;
			}
			currentOrg.set(orgData);

			if (!repoData || repoData.error) {
				error = 'Repository not found';
				loading = false;
				return;
			}
			repoInfo = repoData;
			currentRepo.set(repoData);

			if (allRefsData?.refs) {
				snapshotCount = allRefsData.refs.filter(r => r.name.startsWith('snap.')).length;
				changesetCount = allRefsData.refs.filter(r => r.name.startsWith('cs.')).length;
			}

			if (reviewsData?.reviews) {
				reviewCount = reviewsData.reviews.length;
			}

			// Find latest snapshot
			const refsData = { refs: allRefsData?.refs?.filter(r => r.name.startsWith('snap.')) || [] };
			if (refsData.refs.length > 0) {
				// Find snap.latest or most recent
				const latestRef = refsData.refs.find(r => r.name === 'snap.latest') || refsData.refs[0];
				if (latestRef) {
					// Decode base64 target to hex
					const targetBytes = atob(latestRef.target);
					latestSnapshot = Array.from(targetBytes, b => b.charCodeAt(0).toString(16).padStart(2, '0')).join('');
				}
			}

			if (latestSnapshot) {
				// Load files to find README
				const filesData = await api('GET', `/${slug}/${repo}/v1/files/${latestSnapshot}`);
				if (filesData?.files) {
					allFiles = filesData.files;
					fileCount = filesData.files.length;
					// Build path->digest map for resolving image URLs in markdown
					const map = {};
					for (const f of filesData.files) {
						map[f.path] = f.digest;
					}
					fileDigestMap = map;
					const readme = filesData.files.find(f => isReadme(f.path) && !f.path.includes('/')) || filesData.files.find(f => isReadme(f.path));
					if (readme) {
						readmeFile = readme;
						// Load README content
						const contentData = await api('GET', `/${slug}/${repo}/v1/content/${readme.digest}`);
						if (contentData?.content) {
							try {
								readmeContent = base64ToUtf8(contentData.content);
							} catch {
								readmeContent = '';
							}
						}
					}
				}
			}
		} catch (e) {
			console.error('Failed to load repo data:', e);
			error = 'Failed to load repository';
		}

		loading = false;
	}
</script>

<svelte:head>
	<title>{repo} - {slug} | Kai</title>
</svelte:head>

<div class="max-w-6xl mx-auto px-5 py-8">
	<RepoNav active="readme" />

	{#if loading}
		<!-- Skeleton loader -->
		<div class="animate-pulse">
			<div class="h-4 bg-kai-bg-tertiary rounded w-1/3 mb-4"></div>
			<div class="space-y-3">
				<div class="h-3 bg-kai-bg-tertiary rounded w-full"></div>
				<div class="h-3 bg-kai-bg-tertiary rounded w-5/6"></div>
				<div class="h-3 bg-kai-bg-tertiary rounded w-4/6"></div>
				<div class="h-3 bg-kai-bg-tertiary rounded w-full"></div>
				<div class="h-3 bg-kai-bg-tertiary rounded w-3/4"></div>
			</div>
			<div class="h-32 bg-kai-bg-tertiary rounded mt-6"></div>
		</div>
	{:else if error}
		<div class="text-center py-16 px-8">
			<div class="mb-4 flex justify-center">
				<svg class="w-12 h-12 text-kai-text-muted" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
					<path stroke-linecap="round" stroke-linejoin="round" d="M12 9v3.75m-9.303 3.376c-.866 1.5.217 3.374 1.948 3.374h14.71c1.73 0 2.813-1.874 1.948-3.374L13.949 3.378c-.866-1.5-3.032-1.5-3.898 0L2.697 16.126ZM12 15.75h.007v.008H12v-.008Z" />
				</svg>
			</div>
			<p class="font-medium text-kai-text mb-1">{error}</p>
			<p class="text-sm text-kai-text-muted mb-6">Check the URL or your access permissions.</p>
			<a href="/{slug}" class="btn btn-primary no-underline">Back to {slug}</a>
		</div>
	{:else}
		<div class="flex gap-8">
			<!-- Main content -->
			<div class="flex-1 min-w-0">
				<!-- README -->
				{#if readmeContent}
					<div class="relative group/readme">
						<!-- svelte-ignore a11y_click_events_have_key_events -->
						<!-- svelte-ignore a11y_no_static_element_interactions -->
						<div class="readme-content" onclick={handleMarkdownClick}>
							<div class="markdown-body">
								{@html safeMarkdown(readmeContent)}
							</div>
						</div>
					</div>
		{:else if latestSnapshot}
			<div class="text-center py-16 px-8">
				<div class="mb-4 flex justify-center">
					<svg class="w-12 h-12 text-kai-text-muted" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
						<path stroke-linecap="round" stroke-linejoin="round" d="M19.5 14.25v-2.625a3.375 3.375 0 0 0-3.375-3.375h-1.5A1.125 1.125 0 0 1 13.5 7.125v-1.5a3.375 3.375 0 0 0-3.375-3.375H8.25m0 12.75h7.5m-7.5 3H12M10.5 2.25H5.625c-.621 0-1.125.504-1.125 1.125v17.25c0 .621.504 1.125 1.125 1.125h12.75c.621 0 1.125-.504 1.125-1.125V11.25a9 9 0 0 0-9-9Z" />
					</svg>
				</div>
				<p class="font-medium text-kai-text mb-1">Help others understand this project</p>
				<p class="text-sm text-kai-text-muted mb-6">Add a README to describe what this repository is about.</p>
				<div class="flex items-center justify-center gap-3">
					<a href="/{slug}/{repo}/snap.latest" class="btn no-underline">Browse files</a>
				</div>
			</div>
				{:else}
					<div class="text-center py-16 px-8">
						<div class="mb-4 flex justify-center">
							<svg class="w-12 h-12 text-kai-text-muted" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
								<path stroke-linecap="round" stroke-linejoin="round" d="M20.25 7.5l-.625 10.632a2.25 2.25 0 0 1-2.247 2.118H6.622a2.25 2.25 0 0 1-2.247-2.118L3.75 7.5M10 11.25h4M3.375 7.5h17.25c.621 0 1.125-.504 1.125-1.125v-1.5c0-.621-.504-1.125-1.125-1.125H3.375c-.621 0-1.125.504-1.125 1.125v1.5c0 .621.504 1.125 1.125 1.125Z" />
							</svg>
						</div>
						<p class="font-medium text-kai-text mb-1">This repository is empty</p>
						<p class="text-sm text-kai-text-muted">Push some content to get started.</p>
					</div>
				{/if}
			</div>

			<!-- Sidebar -->
			<div class="w-72 shrink-0 hidden lg:block">
				<div class="space-y-6">
					<!-- About -->
					{#if repoInfo?.description}
						<div>
							<h3 class="text-sm font-semibold text-kai-text mb-2">About</h3>
							<p class="text-sm text-kai-text-muted">{repoInfo.description}</p>
						</div>
					{/if}

					<!-- Resources -->
					<div>
						<h3 class="text-sm font-semibold text-kai-text mb-2">Resources</h3>
						<div class="space-y-1.5">
							<a href="/{slug}/{repo}/snap.latest/README.md" class="flex items-center gap-2 text-sm text-kai-text-muted hover:text-kai-text no-underline">
								<svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M12 6.042A8.967 8.967 0 0 0 6 3.75c-1.052 0-2.062.18-3 .512v14.25A8.987 8.987 0 0 1 6 18c2.305 0 4.408.867 6 2.292m0-14.25a8.966 8.966 0 0 1 6-2.292c1.052 0 2.062.18 3 .512v14.25A8.987 8.987 0 0 0 18 18a8.967 8.967 0 0 0-6 2.292m0-14.25v14.25" /></svg>
								Readme
							</a>
							{#if hasLicense}
								<a href="/{slug}/{repo}/snap.latest/LICENSE" class="flex items-center gap-2 text-sm text-kai-text-muted hover:text-kai-text no-underline">
									<svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M12 3v17.25m0 0c-1.472 0-2.882.265-4.185.75M12 20.25c1.472 0 2.882.265 4.185.75M18.75 4.97A48.416 48.416 0 0 0 12 4.5c-2.291 0-4.545.16-6.75.47m13.5 0c1.01.143 2.01.317 3 .52m-3-.52 2.62 10.726c.122.499-.106 1.028-.589 1.202a5.988 5.988 0 0 1-2.031.352 5.988 5.988 0 0 1-2.031-.352c-.483-.174-.711-.703-.59-1.202L18.75 4.971Zm-16.5.52c.99-.203 1.99-.377 3-.52m0 0 2.62 10.726c.122.499-.106 1.028-.589 1.202a5.989 5.989 0 0 1-2.031.352 5.989 5.989 0 0 1-2.031-.352c-.483-.174-.711-.703-.59-1.202L5.25 4.971Z" /></svg>
									License
								</a>
							{/if}
							{#if hasContributing}
								<a href="/{slug}/{repo}/snap.latest/CONTRIBUTING.md" class="flex items-center gap-2 text-sm text-kai-text-muted hover:text-kai-text no-underline">
									<svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M15 19.128a9.38 9.38 0 0 0 2.625.372 9.337 9.337 0 0 0 4.121-.952 4.125 4.125 0 0 0-7.533-2.493M15 19.128v-.003c0-1.113-.285-2.16-.786-3.07M15 19.128v.106A12.318 12.318 0 0 1 8.624 21c-2.331 0-4.512-.645-6.374-1.766l-.001-.109a6.375 6.375 0 0 1 11.964-3.07M12 6.375a3.375 3.375 0 1 1-6.75 0 3.375 3.375 0 0 1 6.75 0Zm8.25 2.25a2.625 2.625 0 1 1-5.25 0 2.625 2.625 0 0 1 5.25 0Z" /></svg>
									Contributing
								</a>
							{/if}
							{#if hasSecurity}
								<a href="/{slug}/{repo}/snap.latest/SECURITY.md" class="flex items-center gap-2 text-sm text-kai-text-muted hover:text-kai-text no-underline">
									<svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M9 12.75 11.25 15 15 9.75m-3-7.036A11.959 11.959 0 0 1 3.598 6 11.99 11.99 0 0 0 3 9.749c0 5.592 3.824 10.29 9 11.623 5.176-1.332 9-6.03 9-11.622 0-1.31-.21-2.571-.598-3.751h-.152c-3.196 0-6.1-1.248-8.25-3.285Z" /></svg>
									Security
								</a>
							{/if}
							{#if hasChangelog}
								<a href="/{slug}/{repo}/snap.latest/CHANGELOG.md" class="flex items-center gap-2 text-sm text-kai-text-muted hover:text-kai-text no-underline">
									<svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M8.25 6.75h12M8.25 12h12m-12 5.25h12M3.75 6.75h.007v.008H3.75V6.75Zm.375 0a.375.375 0 1 1-.75 0 .375.375 0 0 1 .75 0ZM3.75 12h.007v.008H3.75V12Zm.375 0a.375.375 0 1 1-.75 0 .375.375 0 0 1 .75 0Zm-.375 5.25h.007v.008H3.75v-.008Zm.375 0a.375.375 0 1 1-.75 0 .375.375 0 0 1 .75 0Z" /></svg>
									Changelog
								</a>
							{/if}
						</div>
					</div>

					<!-- Activity -->
					<div>
						<h3 class="text-sm font-semibold text-kai-text mb-2">Activity</h3>
						<div class="space-y-1.5 text-sm text-kai-text-muted">
							<div class="flex items-center gap-2">
								<svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M19.5 14.25v-2.625a3.375 3.375 0 0 0-3.375-3.375h-1.5A1.125 1.125 0 0 1 13.5 7.125v-1.5a3.375 3.375 0 0 0-3.375-3.375H8.25m2.25 0H5.625c-.621 0-1.125.504-1.125 1.125v17.25c0 .621.504 1.125 1.125 1.125h12.75c.621 0 1.125-.504 1.125-1.125V11.25a9 9 0 0 0-9-9Z" /></svg>
								{fileCount} files
							</div>
							<div class="flex items-center gap-2">
								<svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M6.827 6.175A2.31 2.31 0 0 1 5.186 7.23c-.38.054-.757.112-1.134.175C2.999 7.58 2.25 8.507 2.25 9.574V18a2.25 2.25 0 0 0 2.25 2.25h15A2.25 2.25 0 0 0 21.75 18V9.574c0-1.067-.75-1.994-1.802-2.169a47.865 47.865 0 0 0-1.134-.175 2.31 2.31 0 0 1-1.64-1.055l-.822-1.316a2.192 2.192 0 0 0-1.736-1.039 48.774 48.774 0 0 0-5.232 0 2.192 2.192 0 0 0-1.736 1.039l-.821 1.316Z" /><path stroke-linecap="round" stroke-linejoin="round" d="M16.5 12.75a4.5 4.5 0 1 1-9 0 4.5 4.5 0 0 1 9 0Z" /></svg>
								{snapshotCount} snapshots
							</div>
							{#if changesetCount > 0}
								<div class="flex items-center gap-2">
									<svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M7.5 21 3 16.5m0 0L7.5 12M3 16.5h13.5m0-13.5L21 7.5m0 0L16.5 12M21 7.5H7.5" /></svg>
									{changesetCount} changesets
								</div>
							{/if}
							{#if reviewCount > 0}
								<div class="flex items-center gap-2">
									<svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M9 12h3.75M9 15h3.75M9 18h3.75m3 .75H18a2.25 2.25 0 0 0 2.25-2.25V6.108c0-1.135-.845-2.098-1.976-2.192a48.424 48.424 0 0 0-1.123-.08m-5.801 0c-.065.21-.1.433-.1.664 0 .414.336.75.75.75h4.5a.75.75 0 0 0 .75-.75 2.25 2.25 0 0 0-.1-.664m-5.8 0A2.251 2.251 0 0 1 13.5 2.25H15c1.012 0 1.867.668 2.15 1.586m-5.8 0c-.376.023-.75.05-1.124.08C9.095 4.01 8.25 4.973 8.25 6.108V8.25m0 0H4.875c-.621 0-1.125.504-1.125 1.125v11.25c0 .621.504 1.125 1.125 1.125h9.75c.621 0 1.125-.504 1.125-1.125V9.375c0-.621-.504-1.125-1.125-1.125H8.25zM6.75 12h.008v.008H6.75V12zm0 3h.008v.008H6.75V15zm0 3h.008v.008H6.75V18z" /></svg>
									{reviewCount} reviews
								</div>
							{/if}
						</div>
					</div>

					<!-- Languages -->
					{#if languages.length > 0}
						<div>
							<h3 class="text-sm font-semibold text-kai-text mb-2">Languages</h3>
							<div class="flex flex-wrap gap-1.5">
								{#each languages as [lang, count]}
									<span class="px-2 py-0.5 text-xs rounded-full bg-kai-bg-tertiary text-kai-text-muted">{lang}</span>
								{/each}
							</div>
						</div>
					{/if}
				</div>
			</div>
		</div>
	{/if}
</div>

<style>
	/* README content area */
	.readme-content {
		padding: 1.5rem 0;
	}

	/* Image fallback for broken images */
	:global(.img-fallback) {
		display: inline-flex;
		align-items: center;
		gap: 0.5rem;
		padding: 0.75rem 1rem;
		border-radius: 0.5rem;
		background: rgb(var(--kai-bg-tertiary));
		color: rgb(var(--kai-text-muted));
		font-size: 0.875rem;
		margin: 0.5em 0;
	}

	/* Language label on code blocks */
	:global(.code-lang-label) {
		position: absolute;
		top: 0;
		right: 2.5rem;
		font-size: 0.75rem;
		padding: 0.125rem 0.5rem;
		color: rgb(var(--kai-text-muted));
		opacity: 0.7;
		font-family: ui-sans-serif, system-ui, sans-serif;
		z-index: 1;
	}
</style>
