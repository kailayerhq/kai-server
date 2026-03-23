<script>
	let name = $state('');
	let email = $state('');
	let company = $state('');
	let repoUrl = $state('');
	let aiUsage = $state('');
	let submitting = $state(false);
	let submitted = $state(false);
	let error = $state('');

	async function submit() {
		if (!name.trim() || !email.trim()) {
			error = 'Name and email are required.';
			return;
		}
		error = '';
		submitting = true;

		try {
			const resp = await fetch('/api/v1/signups', {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({
					name: name.trim(),
					email: email.trim(),
					company: company.trim(),
					repo_url: repoUrl.trim(),
					ai_usage: aiUsage.trim()
				})
			});

			if (!resp.ok) {
				const data = await resp.json().catch(() => ({}));
				throw new Error(data.error || 'Something went wrong');
			}

			submitted = true;
		} catch (e) {
			error = e.message;
		}

		submitting = false;
	}
</script>

<div class="min-h-screen bg-kai-bg flex items-center justify-center px-4">
	<div class="max-w-lg w-full">
		{#if submitted}
			<div class="text-center py-16">
				<div class="w-16 h-16 mx-auto mb-6 rounded-full bg-green-500/10 flex items-center justify-center">
					<svg class="w-8 h-8 text-green-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7" />
					</svg>
				</div>
				<h1 class="text-2xl font-semibold text-kai-text mb-3">Thanks for signing up</h1>
				<p class="text-kai-text-muted leading-relaxed">
					We're onboarding teams in batches right now.<br>
					We'll reach out soon.
				</p>
				<a href="https://kaicontext.com" class="inline-block mt-8 text-sm text-kai-accent hover:underline">
					Back to kaicontext.com
				</a>
			</div>
		{:else}
			<div class="mb-8 text-center">
				<img src="/favicon-96x96.png" alt="Kai" class="w-10 h-10 mx-auto mb-4" />
				<h1 class="text-2xl font-semibold text-kai-text">Get early access to Kai</h1>
				<p class="text-kai-text-muted mt-2">Semantic infrastructure for code change. CI, reviews, and AI context — powered by what your code means.</p>
			</div>

			<div class="bg-kai-bg-secondary border border-kai-border rounded-lg p-6 space-y-4">
				{#if error}
					<div class="text-sm text-red-500 bg-red-500/10 rounded px-3 py-2">{error}</div>
				{/if}

				<div>
					<label for="name" class="block text-sm font-medium text-kai-text mb-1">Name *</label>
					<input id="name" type="text" bind:value={name} class="input w-full" placeholder="Jane Smith" />
				</div>

				<div>
					<label for="email" class="block text-sm font-medium text-kai-text mb-1">Email *</label>
					<input id="email" type="email" bind:value={email} class="input w-full" placeholder="jane@company.com" />
				</div>

				<div>
					<label for="company" class="block text-sm font-medium text-kai-text mb-1">Company <span class="text-kai-text-muted font-normal">(optional)</span></label>
					<input id="company" type="text" bind:value={company} class="input w-full" placeholder="Acme Corp" />
				</div>

				<div>
					<label for="repo" class="block text-sm font-medium text-kai-text mb-1">Repo / codebase URL <span class="text-kai-text-muted font-normal">(optional)</span></label>
					<input id="repo" type="url" bind:value={repoUrl} class="input w-full" placeholder="https://github.com/org/repo" />
				</div>

				<div>
					<label for="ai" class="block text-sm font-medium text-kai-text mb-1">How are you using AI coding tools today? <span class="text-kai-text-muted font-normal">(optional)</span></label>
					<textarea id="ai" bind:value={aiUsage} class="input w-full" rows="3" placeholder="e.g., Copilot for autocomplete, Claude for refactoring..."></textarea>
				</div>

				<button
					class="btn btn-primary w-full"
					onclick={submit}
					disabled={submitting}
				>
					{submitting ? 'Submitting...' : 'Request Early Access'}
				</button>
			</div>

			<p class="text-center text-xs text-kai-text-muted mt-4">
				Already have access? <a href="/" class="text-kai-accent hover:underline">Sign in</a>
			</p>
		{/if}
	</div>
</div>
