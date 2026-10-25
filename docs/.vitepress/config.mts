import { defineConfig } from 'vitepress';

const repo = 'https://github.com/huynguyengl99/chanx-go';
const pkg = 'https://pkg.go.dev/github.com/huynguyengl99/chanx-go';

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
  base: '/chanx-go/',
  cleanUrls: true,
  // Pages link to the server the reader runs, at localhost.
  ignoreDeadLinks: 'localhostLinks',
  srcExclude: ['snippets/**'],
  // Head links are not prefixed with `base`, so they carry it.
  head: [
    ['meta', { name: 'theme-color', content: '#00758d' }],
    ['link', { rel: 'icon', type: 'image/svg+xml', href: '/chanx-go/logo.svg' }],
  ],
  themeConfig: {
    logo: '/logo.svg',
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
          { text: 'chanx (Python)', link: 'https://chanx.readthedocs.io' },
          { text: 'chanx-js (TypeScript)', link: 'https://huynguyengl99.github.io/chanx-js/' },
          { text: 'chanx-kit', link: 'https://huynguyengl99.github.io/chanx-kit/' },
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
