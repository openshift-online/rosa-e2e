// configgen generates the resolved CI status registry consumed by chai-bot
// tasks and rosa-eng-dashboard:
//
//	configgen resolve   lean registry + vendored org snapshot -> generated registry
//	configgen snapshot  org repo checkout -> vendored org snapshot
//
// Run via `make configs` and `make configs-snapshot`.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/openshift-online/rosa-e2e/pkg/config"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "resolve":
		if err := runResolve(os.Args[2:]); err != nil {
			fail(err)
		}
	case "snapshot":
		if err := runSnapshot(os.Args[2:]); err != nil {
			fail(err)
		}
	default:
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: configgen <resolve|snapshot> [flags]")
}

func fail(err error) {
	fmt.Fprintf(os.Stderr, "configgen: %v\n", err)
	os.Exit(1)
}

func runResolve(args []string) error {
	fs := flag.NewFlagSet("resolve", flag.ExitOnError)
	leanPath := fs.String("lean", "configs/ci-status-jobs.yaml", "lean registry path")
	snapshotPath := fs.String("snapshot", "configs/generated/org-teams.yaml", "org snapshot path")
	outPath := fs.String("out", "configs/generated/ci-status-jobs.yaml", "output path")
	if err := fs.Parse(args); err != nil {
		return err
	}

	lean, err := config.LoadLeanCIStatusConfig(*leanPath)
	if err != nil {
		return err
	}
	snap, err := config.LoadOrgSnapshot(*snapshotPath)
	if err != nil {
		return err
	}
	resolved, err := config.ResolveSnapshot(lean, snap)
	if err != nil {
		return err
	}
	out, err := config.MarshalCIStatus(resolved)
	if err != nil {
		return err
	}
	return os.WriteFile(*outPath, out, 0o644)
}

func runSnapshot(args []string) error {
	fs := flag.NewFlagSet("snapshot", flag.ExitOnError)
	orgPath := fs.String("org", "../org", "org repo checkout path")
	repo := fs.String("repo", "", "canonical repo name for provenance (default: the checkout's origin remote)")
	outPath := fs.String("out", "configs/generated/org-teams.yaml", "output path")
	if err := fs.Parse(args); err != nil {
		return err
	}

	snap, err := config.SnapshotFromOrg(*orgPath)
	if err != nil {
		return err
	}
	if *repo != "" {
		snap.Source.Repo = *repo
	}
	out, err := config.MarshalOrgSnapshot(snap)
	if err != nil {
		return err
	}
	return os.WriteFile(*outPath, out, 0o644)
}
