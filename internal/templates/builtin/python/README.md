# {{TITLE}}

Python project on a pinned interpreter, managed with [codenv](https://github.com/Tijani127/codenv).

## Getting started

```sh
codenv install
codenv shell
codenv run venv
```

Inside the shell, activate the environment:

```powershell
.venv\Scripts\Activate.ps1
```

## Scripts

```sh
codenv run test
codenv run lint
```

`codenv.json` pins the interpreter via `versions/python313`, so nobody ends up on a different
Python. Adjust the version to the one you want; `codenv search python` lists what is available.

Commit `codenv.json` and `codenv.lock.json`; ignore `.codenv/` and `.venv/`.
