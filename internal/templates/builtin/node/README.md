# {{TITLE}}

Node.js project on a pinned LTS runtime, managed with [codenv](https://github.com/Tijani127/codenv).

## Getting started

```sh
codenv install
codenv shell
codenv run install-deps
```

## Scripts

```sh
codenv run build
codenv run test
codenv run lint
```

The runtime comes from the store, not your system Node install, so the version is identical
everywhere.

Commit `codenv.json` and `codenv.lock.json`; ignore `.codenv/` and `node_modules/`.
