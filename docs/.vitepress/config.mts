import { defineConfig } from 'vitepress';

const repo = 'https://github.com/chanx-dev/chanx-go';
const pkg = 'https://pkg.go.dev/go.chanx.dev/v2';

const sidebar = [
  {
    text: 'Introduction',
    items: [
      { text: 'What is chanx-go?', link: '/introduction/' },
      { text: 'Why chanx-go?', link: '/introduction/why' },
      { text: 'Quick start', link: '/introduction/quick-start' },
      { text: 'Tutorial: a chat server', link: '/tutorial/chat-server' },
    ],
  },
  {
    text: 'Guide',
    items: [
      { text: 'Messages and validation', link: '/guide/messages' },
      { text: 'Consumers', link: '/guide/consumers' },
      { text: 'Groups and events', link: '/guide/events' },
      { text: 'Topics', link: '/guide/topics' },
      { text: 'Authentication', link: '/guide/authentication' },
      { text: 'AsyncAPI', link: '/guide/asyncapi' },
      { text: 'Configuration', link: '/guide/configuration' },
      { text: 'Testing', link: '/guide/testing' },
      { text: 'Go client', link: '/guide/go-client' },
    ],
  },
  {
    text: 'Frameworks',
    items: [
      { text: 'Overview', link: '/frameworks/' },
      { text: 'net/http, chi, gorilla/mux', link: '/frameworks/nethttp' },
      { text: 'gin', link: '/frameworks/gin' },
      { text: 'echo', link: '/frameworks/echo' },
      { text: 'Fiber', link: '/frameworks/fiber' },
    ],
  },
  {
    text: 'Layers',
    items: [
      { text: 'Channel layers', link: '/layers/' },
      { text: 'Redis and Python', link: '/layers/redis' },
    ],
  },
  {
    text: 'Reference',
    items: [
      { text: 'API (pkg.go.dev)', link: pkg },
      { text: 'Clients', link: '/reference/clients' },
      { text: 'Differences from Python', link: '/reference/differences' },
      { text: 'The sandbox example', link: '/reference/sandbox' },
      { text: 'Changelog', link: '/reference/changelog' },
    ],
  },
];

export default defineConfig({
  title: 'chanx-go',
  description:
    'Structured WebSockets for Go: typed handlers, validation, groups and topics across processes, and AsyncAPI, on the chanx protocol.',
  cleanUrls: true,
  // Pages link to the server the reader runs, at localhost.
  ignoreDeadLinks: 'localhostLinks',
  srcExclude: ['snippets/**'],
  head: [
    // Vanity import path: go fetches <module path>?go-get=1 and reads this tag, also from
    // the 404 page, so every module under go.chanx.dev resolves to the repository.
    ['meta', { name: 'go-import', content: `go.chanx.dev git ${repo}` }],
    ['meta', { name: 'theme-color', content: '#1e3a8a' }],
    [
      'meta',
      {
        property: 'og:image',
        content: 'https://go.chanx.dev/brand/social-card.png',
      },
    ],
    ['meta', { property: 'og:image:alt', content: 'chanx-go: structured WebSockets for Go' }],
    ['meta', { name: 'twitter:card', content: 'summary_large_image' }],
    ['link', { rel: 'icon', type: 'image/svg+xml', href: '/logo.svg' }],
    ['link', { rel: 'icon', type: 'image/png', sizes: '32x32', href: '/favicon-32.png' }],
    ['link', { rel: 'apple-touch-icon', href: '/apple-touch-icon.png' }],
  ],
  themeConfig: {
    // The wordmark already spells the name, so the text title is hidden.
    logo: {
      light: '/brand/chanx-go-wordmark.svg',
      dark: '/brand/chanx-go-wordmark-dark.svg',
      alt: 'chanx-go',
    },
    siteTitle: false,
    nav: [
      { text: 'Guide', link: '/introduction/', activeMatch: '^/(introduction|tutorial|guide)/' },
      { text: 'Frameworks', link: '/frameworks/', activeMatch: '^/frameworks/' },
      { text: 'Layers', link: '/layers/', activeMatch: '^/layers/' },
      {
        text: 'Reference',
        activeMatch: '^/reference/',
        items: [
          { text: 'API (pkg.go.dev)', link: pkg },
          { text: 'Clients', link: '/reference/clients' },
          { text: 'Differences from Python', link: '/reference/differences' },
          { text: 'Changelog', link: '/reference/changelog' },
        ],
      },
      {
        text: 'Ecosystem',
        items: [
          { text: 'chanx (Python)', link: 'https://python.chanx.dev' },
          { text: 'chanx-js (TypeScript)', link: 'https://js.chanx.dev/' },
          { text: 'chanx-kit', link: 'https://kit.chanx.dev/' },
        ],
      },
    ],
    sidebar,
    search: { provider: 'local' },
    editLink: { pattern: `${repo}/edit/main/docs/:path`, text: 'Edit this page' },
    socialLinks: [{ icon: 'github', link: repo }],
    footer: { message: 'MIT licensed' },
    outline: [2, 3],
  },
});
