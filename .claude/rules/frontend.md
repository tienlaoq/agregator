---
paths:
  - "frontend/**/*.ts"
  - "frontend/**/*.tsx"
---

# Frontend Rules

- React 19: `ref` as a regular prop (no `forwardRef`); `useActionState` replaces `useFormState`; `use()` unwraps Server Component promises
- Server Components are `async`; add `'use client'` only when you need hooks or browser APIs
- All forms: React Hook Form + Zod schema validation
- Server state: TanStack Query (caching, deduplication); client state: Zustand
- SSR is required for all catalog/venue pages — never client-only fetch for SEO-critical routes
- Tailwind v4 utility classes only; no custom CSS unless unavoidable
