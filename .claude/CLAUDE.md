# Agents working on Aego

- Binaries go to `.cache/bin` only. Never run `go build` without `-o` in the repository root, and build test binaries only with `go test -c -o .cache/bin/<name>.test`. `make clean` empties `.cache/bin`; `make ci` fails when the root holds a binary or a `*.test` file (`tools/ci-clean-root.sh`).
- Run the full `make ci` before every push. Between pushes, run the checks that cover the change: the tests of the changed packages, `make lint`, `make depcheck`, the docs check and the golden tests when the UI changed.
