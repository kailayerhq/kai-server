# Why I Built Kai

I used to work at GitLab. Also at Remote. Both fully remote companies. At both places, "CI takes a while" meant at least two hours—and that was when things went well. I've got friends at Shopify and Stripe who tell me two hours sounds pretty good compared to what they deal with.

When your test suite takes longer than flying from New York to Chicago, you start seeing CI differently. It's not just slow. It's actively working against you.

GitLab had thousands of runners across multiple clouds. Remote had the same complexity plus compliance and payroll systems, where a flaky test isn't just annoying—it's a real problem. The test matrix had so many combinations I couldn't count them.

Here's what actually wears you down: it's not the wait. It's the uncertainty.

You change one line in a utility file. You *know* it probably only affects the auth service. But CI doesn't know that. It sees "file changed" and queues up 400 jobs "just in case." Two hours and thousands of dollars later, you've confirmed that yes, your comment change didn't break billing.

I watched this at GitLab—a company that literally makes CI tools—and we were still struggling. We parallelized, sharded, cached ourselves into new problems. Every deploy felt like a gamble. Did we run enough tests? Did some heuristic skip something important?

When you run infrastructure at scale, you realize our tools are basically flying blind. They read files. They hash content. They run tasks. But they have no idea what the code actually *does*.

That's the problem. That's why I built Kai.

Not just to speed up builds—that's the bare minimum. I wanted builds to actually make sense. I wanted to ask "what does this change actually do?" and get an answer based on behavior, not which files got touched.

After sitting through enough two-hour builds that should've taken ten minutes, after managing systems across continents and regulatory zones, I got tired of hearing "that's just how CI is."

It isn't. We can build tools that actually understand the software they're running. That's what I'm doing.

— Jacob Schatz
