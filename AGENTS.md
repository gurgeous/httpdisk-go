# AGENTS

## Workflow

- After any non-Markdown change, run `just check`
- Use `just fmt` instead of `gofmt` directly
- Prefer `just` tasks when they exist
- Use `mv` for file moves/renames; use patches for content edits
- Keep commit/PR messages under 80 chars

## Style

- Be succinct, we value that highly
- Prefer small, direct code with early returns and explicit data flow
- Avoid unnecessary interfaces, clever abstractions, extra globals
- Avoid defensive code for impossible or unsupported cases
- Keep comments brief and useful
- If a file has several one-liners, group them under `// one-liners` at bottom
- Fail fast; prefer clear errors and actionable hints
- Small Go library with a minimal cache CLI, not a framework or service

## Tests

- Keep unit tests deterministic and network-free
- Network responses are recorded in `testdata/` with go-vcr, named after the
  test. Rename the fixture when you rename a test
- Use `assert.`, not `require.`
- Avoid trivial or overly specific tests; assert only meaningful behavior
- Bug fixes should usually add or update a test
