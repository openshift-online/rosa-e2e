package config

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const (
	leanRegistryPath     = "../../configs/ci-status-jobs.yaml"
	orgSnapshotPath      = "../../configs/generated/org-teams.yaml"
	generatedRegistryPath = "../../configs/generated/ci-status-jobs.yaml"
	chaiBotDir           = "../../.chai-bot"

	// The lean raw URL must not appear in chai-bot tasks: they must fetch the
	// generated output, which carries the org team identity.
	leanRawURL = "raw.githubusercontent.com/openshift-online/rosa-e2e/main/configs/ci-status-jobs.yaml"

	// Team-data checks stay warn-only until the org backfill lands
	// (ROSAENG-75792) and no stopgaps remain. Tracked in ROSAENG-75793.
	lintBlockingDate = "2026-11-13"
)

type lintMode struct {
	blocking bool
}

func (m lintMode) report(t *testing.T, format string, args ...any) {
	t.Helper()
	if m.blocking {
		t.Errorf(format, args...)
		return
	}
	t.Logf("WARN: "+format, args...)
}

func loadRegistryFixture(t *testing.T) (*LeanCIStatusConfig, *OrgSnapshot, *CIStatusConfig) {
	t.Helper()
	lean, err := LoadLeanCIStatusConfig(leanRegistryPath)
	if err != nil {
		t.Fatalf("%v", err)
	}
	snap, err := LoadOrgSnapshot(orgSnapshotPath)
	if err != nil {
		t.Fatalf("%v", err)
	}
	resolved, err := ResolveSnapshot(lean, snap)
	if err != nil {
		t.Fatalf("%v", err)
	}
	return lean, snap, resolved
}

func lintIsBlocking(t *testing.T, lean *LeanCIStatusConfig) bool {
	t.Helper()
	date, err := time.Parse("2006-01-02", lintBlockingDate)
	if err != nil {
		t.Fatalf("bad lintBlockingDate: %v", err)
	}
	return time.Now().After(date) && len(lean.Stopgaps) == 0
}

// TestGeneratedRegistryIsUpToDate fails when the committed generated output no
// longer matches the lean source plus org snapshot: run `make configs` and
// commit the result.
func TestGeneratedRegistryIsUpToDate(t *testing.T) {
	_, _, resolved := loadRegistryFixture(t)

	want, err := MarshalCIStatus(resolved)
	if err != nil {
		t.Fatalf("failed to marshal generated registry: %v", err)
	}
	committed, err := os.ReadFile(generatedRegistryPath)
	if err != nil {
		t.Fatalf("failed to read %s: %v", generatedRegistryPath, err)
	}
	if !bytes.Equal(bytes.TrimSpace(committed), bytes.TrimSpace(want)) {
		t.Errorf("committed generated registry is stale: run `make configs` and commit configs/generated/ci-status-jobs.yaml")
	}
}

// TestLeanRegistrySourceRules enforces the rules of the lean source form
// itself. These always block: they are properties of the hand-edited file,
// not of org data quality.
func TestLeanRegistrySourceRules(t *testing.T) {
	lean, _, _ := loadRegistryFixture(t)

	catIDs := map[string]bool{}
	for i, cat := range lean.Categories {
		if cat.ID == "" {
			t.Errorf("category[%d]: missing id", i)
		}
		if catIDs[cat.ID] {
			t.Errorf("duplicate category id: %s", cat.ID)
		}
		catIDs[cat.ID] = true

		if cat.Gating == nil {
			t.Errorf("category %s: gating must be explicit (true or false), not omitted", cat.ID)
		}
		if cat.Team == "" {
			t.Errorf("category %s: missing team (org file name, see configs/generated/org-teams.yaml)", cat.ID)
		}
		if len(cat.Jobs) == 0 {
			t.Errorf("category %s: no jobs defined", cat.ID)
		}
	}

	for name, sg := range lean.Stopgaps {
		if sg.JiraTeamID == "" || sg.DisplayName == "" {
			t.Errorf("stopgap %s: display_name and jira_team_id are required", name)
		}
		if _, err := time.Parse("2006-01-02", sg.RemoveBy); err != nil {
			t.Errorf("stopgap %s: remove_by must be a YYYY-MM-DD date: %v", name, err)
		}
	}
}

// TestLeanRegistryTeamData checks org data quality for every referenced
// team: Jira team ID, a reachable Slack channel, an escalation contact, and
// component ownership. Warn-only until the org backfill (ROSAENG-75792)
// lands and the stopgaps are gone.
func TestLeanRegistryTeamData(t *testing.T) {
	lean, snap, _ := loadRegistryFixture(t)
	mode := lintMode{blocking: lintIsBlocking(t, lean)}

	// Component -> owning teams, for the ownership checks below.
	ownerOf := map[string][]string{}
	for name, team := range snap.Teams {
		if team.JiraTeamID == "" {
			continue
		}
		for _, comp := range team.Components {
			ownerOf[comp] = append(ownerOf[comp], name)
		}
	}

	for _, cat := range lean.Categories {
		team, inSnapshot := snap.Teams[cat.Team]
		if sg, isStopgap := lean.Stopgaps[cat.Team]; isStopgap || !inSnapshot {
			// Stopgaps carry their own identity; only the remove_by date is checked.
			if isStopgap {
				if date, err := time.Parse("2006-01-02", sg.RemoveBy); err == nil && time.Now().After(date) {
					mode.report(t, "stopgap %s is past remove_by %s: the org team should exist by now (ROSAENG-75792)", cat.Team, sg.RemoveBy)
				}
			}
			continue
		}

		if team.JiraTeamID == "" {
			t.Errorf("category %s: org team %s has no Jira Team resource, and no stopgap covers it: Jira routing is broken", cat.ID, cat.Team)
		}
		if cat.SlackChannel == "" && team.SlackChannel == "" {
			mode.report(t, "category %s: team %s has no Slack channel in org or lean override, failures cannot be routed", cat.ID, cat.Team)
		}
		if team.Escalation.Person == "" {
			mode.report(t, "category %s: team %s has no team_lead or manager in org, ladder escalation has no contact", cat.ID, cat.Team)
		}
		for _, comp := range cat.Components {
			owners := ownerOf[comp]
			if !contains(owners, cat.Team) {
				mode.report(t, "category %s: component %s is not owned by team %s in org (owners: %v)", cat.ID, comp, cat.Team, owners)
			}
		}
	}
}

// TestChaiBotTasksFetchResolvedRegistry keeps chai-bot tasks pointed at the
// generated output instead of the lean source.
func TestChaiBotTasksFetchResolvedRegistry(t *testing.T) {
	entries, err := os.ReadDir(chaiBotDir)
	if err != nil {
		t.Fatalf("failed to read %s: %v", chaiBotDir, err)
	}
	found := false
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		found = true
		data, err := os.ReadFile(filepath.Join(chaiBotDir, entry.Name()))
		if err != nil {
			t.Fatalf("failed to read %s: %v", entry.Name(), err)
		}
		if strings.Contains(string(data), leanRawURL) {
			t.Errorf("%s references the lean registry raw URL: chai-bot tasks must fetch configs/generated/ci-status-jobs.yaml", entry.Name())
		}
	}
	if !found {
		t.Fatalf("no chai-bot tasks found under %s", chaiBotDir)
	}
}

// TestOrgSnapshotShape sanity-checks the vendored snapshot: every team has a
// display name, and components are sorted (SnapshotFromOrg guarantees it).
func TestOrgSnapshotShape(t *testing.T) {
	_, snap, _ := loadRegistryFixture(t)
	if len(snap.Teams) == 0 {
		t.Fatal("org snapshot has no teams")
	}
	for name, team := range snap.Teams {
		if team.DisplayName == "" {
			t.Errorf("org team %s: missing display_name", name)
		}
		if !sortedStrings(team.Components) {
			t.Errorf("org team %s: components not sorted (regenerate with `make configs-snapshot`)", name)
		}
	}
}

func contains(list []string, s string) bool {
	for _, item := range list {
		if item == s {
			return true
		}
	}
	return false
}

func sortedStrings(list []string) bool {
	for i := 1; i < len(list); i++ {
		if list[i-1] > list[i] {
			return false
		}
	}
	return true
}
