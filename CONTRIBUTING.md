# Contributing

Thanks for looking at Agentgram. For anything non-trivial, **open an issue first** so we can agree on the approach.

## Development

```bash
make install
make docker-up    # API, mock agent, Redis, Postgres, web
make test
make lint
```

Laptop mode (embedded Redis + Postgres):

```bash
cd api && CONFIG_PATH=configs/config.laptop.yaml go run ./cmd/server
```

## Pull requests

- Keep PRs focused.
- Conventional Commits: `feat:`, `fix:`, `docs:`, `test:`, `chore:`.
- Run `make test` and `make lint` before pushing.
- Add or update tests for behaviour you change.

## Good first issues

Look for the `good-first-issue` label. If none are open, documentation and getting-started gaps are always welcome.

## Code of conduct

Be kind. Harassment is not tolerated. The maintainers may refuse or revert contributions that violate that.
