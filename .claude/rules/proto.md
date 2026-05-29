---
paths:
  - "proto/**/*.proto"
---

# Proto Rules

- After any `.proto` change run `make proto-gen` before building — generated code lands in `gen/go/`
- File layout: `proto/{service}/v1/{service}.proto` (service definition), `{model}.proto` (messages)
- One service per `.proto` file; keep message names consistent with domain entity names
- Do not hand-edit files in `gen/go/` — they are always overwritten by `make proto-gen`
