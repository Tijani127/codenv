# {{TITLE}}

Isolated development environment managed with [codenv](https://github.com/Tijani127/codenv).

## Getting started

```sh
codenv install   # install the pinned packages
codenv shell     # enter the environment
codenv list      # show packages and versions
```

## Scripts

Run a script defined in `codenv.json`:

```sh
codenv run <script>
```

Commit `codenv.json` and `codenv.lock.json`. Ignore the local store:

```gitignore
.codenv/
```