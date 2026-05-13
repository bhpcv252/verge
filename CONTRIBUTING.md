# Contributing to Verge

Thanks for wanting to contribute to Verge, a version control system for non-code assets.

---

## What kind of help is useful right now?

As this project is in early development phase, the following help is much appreciated.

**Documentation** - The documentation is almost none currently, please feel free open issue to discuss and work on it.

**Feature requests** - open an issue and describe what you're trying to do and why the current API doesn't support it. Be specific about your use case. "I'm building X and I need Y because Z" is a lot more useful than a vague feature name.

**Bug reports** are always welcome. If something is broken, open an issue and describe what you sent, what came back, and what you expected instead. A minimal reproduction case helps a lot.

**Code contributions** - read the section below before starting anything significant.

---

## Before you start building something

Please open an issue before starting any non-trivial code change. It avoids the frustrating situation where you put real time into something that doesn't align with where the project is heading, or that someone else is already working on.

Things most likely to get merged:

- Bug fixes with a clear reproduction case
- Documentation
- New storage backend implementations that follow the pluggable backend interface
- SDK contributions in TypeScript, Python, or Go (coordinate in an issue first so work isn't duplicated)
- Test coverage for existing flows

---

## Getting started

### Prerequisites

- **Go 1.25.0 or later** - Check with `go version`
- **Docker & Docker Compose** - Required for running PostgreSQL, Redis, Neo4j, and build tools
- **Git** - For version control
- **Make** - For running build commands

### Initial setup

```bash
# Clone the repository
git clone https://github.com/bhpcv252/verge.git
cd verge

# Install git hooks (optional but recommended)
# This runs lint, tests, and formatting before commits
# Install lefthook first: https://github.com/evilmartians/lefthook
lefthook install

# Download Go dependencies
go mod download

# Generate protobuf files
make proto

# Start PostgreSQL (required for local development)
docker compose up -d postgres

# Run database migrations
make migrate-up

# Verify everything works
make test
```

### Running locally

**Option 1: Docker Compose (recommended for full stack)**

```bash
# Start all services (PostgreSQL, Redis, Neo4j, API server, worker)
make up

# API server will be available at:
# - HTTP: http://localhost:8080
# - gRPC: http://localhost:9090
# - Health: http://localhost:8081/health

# Stop all services
make down
```

**Option 2: Native Go (for faster iteration)**

```bash
# Start PostgreSQL only
docker compose up -d postgres

# Run migrations
make migrate-up

# Run the API server
go run cmd/server/main.go

# In another terminal, run the outbox worker
go run cmd/worker/main.go
```

---

## Opening a pull request

1. Fork the repo and create a branch off `main`. Give it a name that says what it does: `fix/branch-conflict-response`, `feat/typescript-sdk`, `docs/grpc-examples`.
2. Keep your commits focused. One logical change per commit makes reviews easier.
3. If you changed API behavior, update the relevant docs.
4. If you changed how something works, add or update tests.
5. Open a PR against `main`. Describe what the change does, why it's needed, and how to test it.

---

## A few code conventions

- Match the style of whatever file you're editing
- Keep functions focused on one thing
- Handle errors explicitly
- Any new API behavior needs a structured error response with a machine-readable `error` code and a human-readable `message`

---

## Commit messages

Write them in the imperative: `Add idempotency key support to commit creation`, not `Added` or `Adds`. Keep the subject line under 72 characters.
If the change needs more explanation, add a body after a blank line.

---

## Security issues

Please don't open a public GitHub issue for security vulnerabilities. [Email](mailto:sayhellotosonu@gmail.com) the maintainers directly instead.

---

## License

By contributing, you agree your changes will be licensed under the MIT License.
