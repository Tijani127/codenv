# {{TITLE}}

Go project pinned to an exact toolchain with [codenv](https://github.com/Tijani127/codenv).

## Getting started

```sh
codenv install   # installs Go and everything else in codenv.json
codenv shell     # go is on PATH, from the store, not your system
```

## Scripts

```sh
codenv run build   # go build -o bin/{{PROJECT}}.exe .
codenv run test    # go test ./...
codenv run vet     # go vet ./...
codenv run fmt     # gofmt -w .
codenv run check   # vet, then test
```

Because the Go version is pinned in `codenv.json`, every machine and CI run uses the same compiler.

Commit `codenv.json` and `codenv.lock.json`; ignore `.codenv/`.