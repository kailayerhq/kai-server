import { defineConfig } from 'vitepress'

export default defineConfig({
  title: 'Kai',
  description: 'Semantic code intelligence for CI/CD',
  base: '/',
  ignoreDeadLinks: [
    /\.\.\/LICENSE/,
    /\.\.\/SECURITY/,
    /\.\.\/CONTRIBUTING/,
  ],

  head: [
    ['link', { rel: 'icon', href: '/favicon.ico' }],
  ],

  themeConfig: {
    nav: [
      { text: 'Docs', link: '/' },
      { text: 'GitHub', link: 'https://github.com/kaicontext/kai' },
      { text: 'kaicontext.com', link: 'https://kaicontext.com' },
    ],

    sidebar: [
      {
        text: 'Getting Started',
        items: [
          { text: 'Introduction', link: '/' },
          { text: 'Why I Built Kai', link: '/why' },
          { text: 'CLI Reference', link: '/cli-reference' },
          { text: 'How Kai Handles Your Code', link: '/data-handling' },
        ],
      },
      {
        text: 'CI',
        items: [
          { text: 'Kailab CI Workflows', link: '/ci-workflows' },
          { text: 'GitHub Action', link: '/github-action' },
          { text: 'GitLab CI', link: '/gitlab-ci' },
        ],
      },
      {
        text: 'Architecture',
        items: [
          { text: 'OSS vs Cloud', link: '/architecture-boundary' },
          { text: 'Boundary Spec', link: '/boundary-spec' },
          { text: 'Extension Points', link: '/extension-points' },
        ],
      },
      {
        text: 'Project',
        items: [
          { text: 'Licensing', link: '/licensing' },
          { text: 'Patent Posture', link: '/patent-posture' },
          { text: 'IP Ownership', link: '/ip-ownership' },
          { text: 'Changelog', link: '/changelog' },
        ],
      },
    ],

    socialLinks: [
      { icon: 'github', link: 'https://github.com/kaicontext/kai' },
    ],

    search: {
      provider: 'local',
    },
  },
})
