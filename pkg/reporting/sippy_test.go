package reporting

import (
	"encoding/xml"
	"os"
	"path/filepath"
	"testing"
)

func TestWriteSippyJUnit_AllPassNoUpgrade(t *testing.T) {
	result := SignalResult{
		InstallPassed:        true,
		InfrastructurePassed: true,
	}

	path := filepath.Join(t.TempDir(), "junit.xml")
	if err := WriteSippyJUnit(result, path); err != nil {
		t.Fatal(err)
	}

	suites := readSuites(t, path)
	if suites.Tests != 2 {
		t.Errorf("expected 2 tests, got %d", suites.Tests)
	}
	if suites.Failures != 0 {
		t.Errorf("expected 0 failures, got %d", suites.Failures)
	}

	assertTestCase(t, suites, 0, InstallTestName, "passed")
	assertTestCase(t, suites, 1, InfrastructureTestName, "passed")
}

func TestWriteSippyJUnit_AllPassWithUpgrade(t *testing.T) {
	result := SignalResult{
		InstallPassed:        true,
		InfrastructurePassed: true,
		UpgradeRan:           true,
		UpgradePassed:        true,
	}

	path := filepath.Join(t.TempDir(), "junit.xml")
	if err := WriteSippyJUnit(result, path); err != nil {
		t.Fatal(err)
	}

	suites := readSuites(t, path)
	if suites.Tests != 3 {
		t.Errorf("expected 3 tests, got %d", suites.Tests)
	}
	if suites.Failures != 0 {
		t.Errorf("expected 0 failures, got %d", suites.Failures)
	}

	assertTestCase(t, suites, 2, UpgradeTestName, "passed")
}

func TestWriteSippyJUnit_InfrastructureFail(t *testing.T) {
	result := SignalResult{
		InstallPassed:        true,
		InfrastructurePassed: false,
		InfraFailureMsg:      "ClusterOperator monitoring is degraded",
	}

	path := filepath.Join(t.TempDir(), "junit.xml")
	if err := WriteSippyJUnit(result, path); err != nil {
		t.Fatal(err)
	}

	suites := readSuites(t, path)
	if suites.Failures != 1 {
		t.Errorf("expected 1 failure, got %d", suites.Failures)
	}

	tc := suites.Suites[0].Cases[1]
	if tc.Status != "failed" {
		t.Errorf("expected infra test to be failed, got %s", tc.Status)
	}
	if tc.Failure == nil {
		t.Fatal("expected failure element on infra test")
	}
	if tc.Failure.Message != "ClusterOperator monitoring is degraded" {
		t.Errorf("unexpected failure message: %s", tc.Failure.Message)
	}
}

func TestWriteSippyJUnit_UpgradeFail(t *testing.T) {
	result := SignalResult{
		InstallPassed:        true,
		InfrastructurePassed: true,
		UpgradeRan:           true,
		UpgradePassed:        false,
		UpgradeFailureMsg:    "upgrade timed out after 60m",
	}

	path := filepath.Join(t.TempDir(), "junit.xml")
	if err := WriteSippyJUnit(result, path); err != nil {
		t.Fatal(err)
	}

	suites := readSuites(t, path)
	if suites.Failures != 1 {
		t.Errorf("expected 1 failure, got %d", suites.Failures)
	}

	tc := suites.Suites[0].Cases[2]
	if tc.Failure == nil || tc.Failure.Message != "upgrade timed out after 60m" {
		t.Error("expected upgrade failure with timeout message")
	}
}

func TestWriteSippyJUnit_InstallFail(t *testing.T) {
	result := SignalResult{
		InstallPassed:        false,
		InfrastructurePassed: false,
		InstallFailureMsg:    "BeforeSuite failed: cannot connect to OCM",
		InfraFailureMsg:      "cluster unreachable",
	}

	path := filepath.Join(t.TempDir(), "junit.xml")
	if err := WriteSippyJUnit(result, path); err != nil {
		t.Fatal(err)
	}

	suites := readSuites(t, path)
	if suites.Failures != 2 {
		t.Errorf("expected 2 failures, got %d", suites.Failures)
	}

	tc := suites.Suites[0].Cases[0]
	if tc.Status != "failed" {
		t.Errorf("expected install test to be failed, got %s", tc.Status)
	}
}

func TestWriteSippyJUnit_UpgradeNotEmittedWhenNotRun(t *testing.T) {
	result := SignalResult{
		InstallPassed:        true,
		InfrastructurePassed: true,
		UpgradeRan:           false,
	}

	path := filepath.Join(t.TempDir(), "junit.xml")
	if err := WriteSippyJUnit(result, path); err != nil {
		t.Fatal(err)
	}

	suites := readSuites(t, path)
	for _, tc := range suites.Suites[0].Cases {
		if tc.Name == UpgradeTestName {
			t.Error("upgrade test should not be emitted when UpgradeRan is false")
		}
	}
}

func TestWriteSippyJUnit_ExactTestNames(t *testing.T) {
	result := SignalResult{
		InstallPassed:        true,
		InfrastructurePassed: true,
		UpgradeRan:           true,
		UpgradePassed:        true,
	}

	path := filepath.Join(t.TempDir(), "junit.xml")
	if err := WriteSippyJUnit(result, path); err != nil {
		t.Fatal(err)
	}

	suites := readSuites(t, path)
	expected := []string{
		"install should succeed: overall",
		"install should succeed: infrastructure",
		"[sig-sippy] upgrade should work",
	}
	for i, tc := range suites.Suites[0].Cases {
		if tc.Name != expected[i] {
			t.Errorf("testcase %d: expected name %q, got %q", i, expected[i], tc.Name)
		}
	}
}

func readSuites(t *testing.T, path string) junitTestSuites {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var suites junitTestSuites
	if err := xml.Unmarshal(data, &suites); err != nil {
		t.Fatal(err)
	}
	if len(suites.Suites) != 1 {
		t.Fatalf("expected 1 suite, got %d", len(suites.Suites))
	}
	return suites
}

func assertTestCase(t *testing.T, suites junitTestSuites, idx int, name, status string) {
	t.Helper()
	if idx >= len(suites.Suites[0].Cases) {
		t.Fatalf("testcase index %d out of range (have %d)", idx, len(suites.Suites[0].Cases))
	}
	tc := suites.Suites[0].Cases[idx]
	if tc.Name != name {
		t.Errorf("testcase %d: expected name %q, got %q", idx, name, tc.Name)
	}
	if tc.Status != status {
		t.Errorf("testcase %d (%s): expected status %q, got %q", idx, name, status, tc.Status)
	}
}
