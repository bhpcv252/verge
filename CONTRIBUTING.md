# Contributing to Verge

Thanks for wanting to contribute. This is a pretty straightforward guide - what's useful, how the process works, and what to expect.

---

## What kind of help is useful right now?

**Bug reports** are always welcome. If something is broken, open an issue and describe what you sent, what came back, and what you expected instead. A minimal reproduction case helps a lot.

**Documentation fixes** are low-friction and high-value. If something in the docs confused you, it'll confuse the next person too. The docs all live as `.md` files at the root of the repo, so a PR to fix something is easy to review and merge quickly.

**Feature requests** - open an issue and describe what you're trying to do and why the current API doesn't support it. Be specific about your use case. "I'm building X and I need Y because Z" is a lot more useful than a vague feature name.

**Code contributions** - read the section below before starting anything significant. The short version: open an issue first.

---

## Before you start building something

Please open an issue before starting any non-trivial code change. It avoids the frustrating situation where you put real time into something that doesn't align with where the project is heading, or that someone else is already working on.

Things most likely to get merged:

- Bug fixes with a clear reproduction case
- Backward-compatible improvements to existing API behavior
- New storage backend implementations that follow the pluggable backend interface
- SDK contributions in TypeScript, Python, or Go (coordinate in an issue first so work isn't duplicated)
- Test coverage for existing flows

---

## Dev setup

```bash
# Clone the repo
git clone https://github.com/your-org/verge.git
cd verge

# Install dependencies
# (language-specific instructions coming once the repo structure is settled)

# Run tests
# (test command coming soon)

# Start a local instance
# (PostgreSQL is the only required dependency to run locally)
```

---

## Opening a pull request

1. Fork the repo and create a branch off `main`. Give it a name that says what it does: `fix/branch-conflict-response`, `feat/typescript-sdk`, `docs/grpc-examples`.
2. Keep your commits focused. One logical change per commit makes reviews easier.
3. If you changed API behavior, update the relevant docs.
4. If you changed how something works, add or update tests.
5. Open a PR against `main`. Describe what the change does, why it's needed, and how to test it.

We try to give an initial response within a few days. Expect feedback - that's just how reviews work and it's not personal.

---

## A few code conventions

- Match the style of whatever file you're editing
- Keep functions focused on one thing
- Handle errors explicitly
- Any new API behavior needs a structured error response with a machine-readable `error` code and a human-readable `message`

---

## Commit messages

Write them in the imperative: `Add idempotency key support to commit creation`, not `Added` or `Adds`. Keep the subject line under 72 characters. If the change needs more explanation, add a body after a blank line.

---

## Security issues

Please don't open a public GitHub issue for security vulnerabilities. Email the maintainers directly instead (contact details will be listed here). We'll respond quickly and coordinate a fix before anything is disclosed publicly.

---

## License

By contributing, you agree your changes will be licensed under the MIT License.
