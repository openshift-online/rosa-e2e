# Generated registries

Copies of the config registries in `configs/`, embedded into the module via
`go:embed` so Go consumers (rosa-eng-dashboard, others) read them through the
vendored module instead of hand-copying yaml or fetching moving files from
main.

## Recipe

Adding or extending a registry follows the same five steps:

1. Hand-authored source yaml lives in `configs/` (e.g.
   `configs/component-deployments.yaml`).
2. `make configs` syncs it into this directory (`cp` in the Makefile).
3. An accessor file in `pkg/config` embeds it and exposes two functions:
   `GeneratedX() []byte` (raw yaml, defensive copy) and
   `ParseGeneratedX()` (typed, same struct as the `LoadX` loader).
4. A lint test asserts the embedded copy matches the committed source and
   parses cleanly, so drift fails CI instead of shipping stale data.
5. Run `make configs` and commit the result whenever the source yaml changes.

Consumers vendor this module; data and parser always come from the same
commit. Raw-URL consumers (chai-bot) fetch the generated files from GitHub
instead.
