<script>
	import '../app.css';
	import { currentUser } from '$lib/stores.js';
	import { loadUser, logout } from '$lib/api.js';
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/stores';

	let { children } = $props();
	let showUserMenu = $state(false);
	let version = $state('');

	// Theme state: 'light', 'dark', or 'system'
	let themePreference = $state('system');
	let resolvedTheme = $state('light');

	function applyTheme(theme) {
		resolvedTheme = theme;
		document.documentElement.setAttribute('data-theme', theme);
	}

	function cycleTheme() {
		if (themePreference === 'system') {
			themePreference = 'light';
			localStorage.setItem('theme', 'light');
			applyTheme('light');
		} else if (themePreference === 'light') {
			themePreference = 'dark';
			localStorage.setItem('theme', 'dark');
			applyTheme('dark');
		} else {
			themePreference = 'system';
			localStorage.removeItem('theme');
			const prefersDark = window.matchMedia('(prefers-color-scheme: dark)').matches;
			applyTheme(prefersDark ? 'dark' : 'light');
		}
	}

	onMount(async () => {
		// Initialize theme state from what the inline script already set
		const saved = localStorage.getItem('theme');
		if (saved === 'light' || saved === 'dark') {
			themePreference = saved;
			resolvedTheme = saved;
		} else {
			themePreference = 'system';
			resolvedTheme = document.documentElement.getAttribute('data-theme') || 'light';
		}

		// Listen for OS preference changes when in system mode
		const mq = window.matchMedia('(prefers-color-scheme: dark)');
		const handler = (e) => {
			if (themePreference === 'system') {
				applyTheme(e.matches ? 'dark' : 'light');
			}
		};
		mq.addEventListener('change', handler);

		// Try to load user from cookie-based session
		if (!$currentUser) {
			await loadUser();
		}

		// Fetch version from health endpoint
		try {
			const res = await fetch('/health');
			if (res.ok) {
				const data = await res.json();
				version = data.version || '';
			}
		} catch (e) {
			// ignore
		}

		return () => mq.removeEventListener('change', handler);
	});

	function handleLogout() {
		showUserMenu = false;
		logout();
	}

	function goToDashboard() {
		goto('/');
	}

	function goToTokens() {
		showUserMenu = false;
		goto('/tokens');
	}

	function goToSSHKeys() {
		showUserMenu = false;
		goto('/ssh-keys');
	}

	function handleClickOutside(event) {
		if (showUserMenu && !event.target.closest('.user-menu-container')) {
			showUserMenu = false;
		}
	}
</script>

<svelte:window onclick={handleClickOutside} />

{#if $currentUser}
	<header class="bg-kai-bg-secondary" style="box-shadow: 0 1px 3px rgba(0,0,0,0.04);">
		<div class="max-w-6xl mx-auto px-5 flex justify-between items-center h-14">
			<a href="/" class="flex items-center gap-2 text-kai-text hover:text-kai-accent no-underline">
				<img src="/favicon-96x96.png" alt="Kai" class="w-6 h-6" />
				<span class="text-lg font-semibold">Kai</span>
			</a>
			<nav class="flex gap-6 items-center text-sm">
				<button
					onclick={goToDashboard}
					class="text-kai-text-muted hover:text-kai-text no-underline px-2 py-1.5 rounded-md hover:bg-kai-bg transition-colors"
				>
					Dashboard
				</button>
				<a
					href="https://docs.kaicontext.com"
					target="_blank"
					rel="noopener noreferrer"
					class="text-kai-text-muted hover:text-kai-text no-underline px-2 py-1.5 rounded-md hover:bg-kai-bg transition-colors"
				>
					Docs
				</a>
				<!-- Theme toggle -->
				<button
					onclick={cycleTheme}
					class="w-8 h-8 flex items-center justify-center rounded-md text-kai-text-muted hover:text-kai-text hover:bg-kai-bg transition-colors"
					title={themePreference === 'system' ? 'Theme: System' : themePreference === 'light' ? 'Theme: Light' : 'Theme: Dark'}
				>
					{#if themePreference === 'system'}
						<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
							<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9.75 17L9 20l-1 1h8l-1-1-.75-3M3 13h18M5 17h14a2 2 0 002-2V5a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z" />
						</svg>
					{:else if themePreference === 'light'}
						<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
							<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 3v1m0 16v1m9-9h-1M4 12H3m15.364 6.364l-.707-.707M6.343 6.343l-.707-.707m12.728 0l-.707.707M6.343 17.657l-.707.707M16 12a4 4 0 11-8 0 4 4 0 018 0z" />
						</svg>
					{:else}
						<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
							<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M20.354 15.354A9 9 0 018.646 3.646 9.003 9.003 0 0012 21a9.003 9.003 0 008.354-5.646z" />
						</svg>
					{/if}
				</button>
				<!-- User avatar dropdown -->
				<div class="relative user-menu-container">
					<button
						onclick={() => showUserMenu = !showUserMenu}
						class="w-8 h-8 rounded-full bg-kai-text-muted/20 text-kai-text flex items-center justify-center font-semibold text-xs hover:ring-2 hover:ring-kai-accent/30 transition-all"
					>
						{$currentUser.email[0].toUpperCase()}
					</button>
					{#if showUserMenu}
						<div class="absolute right-0 mt-2 w-48 bg-kai-bg-secondary border border-kai-border rounded-md shadow-lg py-1 z-50">
							<div class="px-4 py-2 border-b border-kai-border">
								<p class="text-sm font-medium truncate">{$currentUser.email}</p>
							</div>
							<button
								onclick={goToTokens}
								class="w-full text-left px-4 py-2 text-sm text-kai-text hover:bg-kai-bg transition-colors"
							>
								API Tokens
							</button>
							<button
								onclick={goToSSHKeys}
								class="w-full text-left px-4 py-2 text-sm text-kai-text hover:bg-kai-bg transition-colors"
							>
								SSH Keys
							</button>
							<button
								onclick={handleLogout}
								class="w-full text-left px-4 py-2 text-sm text-red-700 dark:text-red-400 hover:bg-kai-bg transition-colors"
							>
								Logout
							</button>
							{#if version}
								<div class="px-4 py-2 border-t border-kai-border">
									<p class="text-xs text-kai-text-muted">v{version}</p>
								</div>
							{/if}
						</div>
					{/if}
				</div>
			</nav>
		</div>
	</header>
{/if}

<main>
	{@render children()}
</main>
