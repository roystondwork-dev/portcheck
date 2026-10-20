# Governance

portcheck is a community open-source project released under the [MIT License](LICENSE).

## Maintainers

| Name | GitHub | Role |
|---|---|---|
| Royston D | [@roystondwork-dev](https://github.com/roystondwork-dev) | Lead maintainer |

Maintainers review and merge pull requests, triage issues, cut releases, and
enforce the [Code of Conduct](CODE_OF_CONDUCT.md).

## Decision making

- Day-to-day changes are decided through pull request review: one maintainer
  approval plus green CI is required to merge.
- Significant design changes (new commands, public API changes in `pkg/`,
  new dependencies, behavior that touches safety guards) are proposed as an
  **Architecture Decision Record** in [`docs/adr/`](docs/adr/) via pull request,
  and stay open for at least 5 days for feedback.
- When consensus can't be reached, the lead maintainer decides.

## Becoming a maintainer

Contributors with a sustained record of high-quality contributions and reviews
may be nominated by an existing maintainer. Nominations are approved by a
majority of current maintainers.

## Compatibility promise

- The CLI's JSON output carries a `schemaVersion`; fields are only removed or
  changed in a new schema version, announced in the changelog.
- Before `v1.0.0`, minor versions may contain breaking changes to `pkg/` APIs;
  they will always be called out in the changelog. From `v1.0.0`, `pkg/` follows
  strict semantic versioning.
