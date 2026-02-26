import { defineConfig } from 'astro/config';
import starlight from '@astrojs/starlight';

export default defineConfig({
  site: 'https://docs.getstronghold.xyz',
  redirects: {
    '/': '/getting-started/',
  },
  integrations: [
    starlight({
      title: 'Stronghold',
      description: 'AI security scanning platform — protect agents from prompt injection and credential leaks',
      social: {
        github: 'https://github.com/yv-was-taken/stronghold',
      },
      head: [
        {
          tag: 'link',
          attrs: {
            rel: 'icon',
            href: '/favicon.svg',
            type: 'image/svg+xml',
          },
        },
      ],
      customCss: [],
      sidebar: [
        {
          label: 'Getting Started',
          items: [
            { label: 'Introduction', slug: 'getting-started' },
            { label: 'Why Network-Level Scanning', slug: 'getting-started/why-network-level' },
            { label: 'Quickstart: Transparent Proxy', slug: 'getting-started/quickstart-proxy' },
            { label: 'Quickstart: Direct API', slug: 'getting-started/quickstart-api' },
            { label: 'Core Concepts', slug: 'getting-started/concepts' },
          ],
        },
        {
          label: 'Transparent Proxy',
          items: [
            { label: 'Installation', slug: 'proxy/installation' },
            { label: 'Setup & Init', slug: 'proxy/setup' },
            { label: 'Enable & Disable', slug: 'proxy/enable-disable' },
            { label: 'Architecture', slug: 'proxy/architecture' },
            { label: 'Response Headers', slug: 'proxy/response-headers' },
            { label: 'Configuration', slug: 'proxy/configuration' },
          ],
        },
        {
          label: 'API Reference',
          items: [
            { label: 'Overview', slug: 'api' },
            { label: 'POST /v1/scan/content', slug: 'api/scan-content' },
            { label: 'POST /v1/scan/output', slug: 'api/scan-output' },
            { label: 'GET /v1/pricing', slug: 'api/pricing' },
            { label: 'Health Checks', slug: 'api/health' },
            { label: 'Errors', slug: 'api/errors' },
          ],
        },
        {
          label: 'Payments & Billing',
          items: [
            { label: 'Pricing', slug: 'billing/pricing' },
            { label: 'x402 Protocol', slug: 'billing/x402' },
            { label: 'Funding Your Account', slug: 'billing/funding' },
          ],
        },
        {
          label: 'Security',
          items: [
            { label: 'Threat Model', slug: 'security/threat-model' },
            { label: 'Detection Layers', slug: 'security/detection-layers' },
          ],
        },
        {
          label: 'Self-Hosting',
          items: [
            { label: 'Overview', slug: 'self-hosting' },
          ],
        },
        {
          label: 'CLI Reference',
          items: [
            { label: 'Overview', slug: 'cli' },
            { label: 'init', slug: 'cli/init' },
            { label: 'enable / disable', slug: 'cli/enable-disable' },
            { label: 'status', slug: 'cli/status' },
            { label: 'health', slug: 'cli/health' },
            { label: 'wallet', slug: 'cli/wallet' },
            { label: 'account', slug: 'cli/account' },
            { label: 'config', slug: 'cli/config' },
            { label: 'doctor', slug: 'cli/doctor' },
          ],
        },
      ],
      components: {},
    }),
  ],
});
