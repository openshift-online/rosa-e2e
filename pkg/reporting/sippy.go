package reporting

import (
	"encoding/xml"
	"os"
)

const (
	InstallTestName        = "install should succeed: overall"
	InfrastructureTestName = "install should succeed: infrastructure"
	UpgradeTestName        = "[sig-sippy] upgrade should work"

	suiteName = "rosa-e2e-sippy-signals"
)

type SignalResult struct {
	InstallPassed        bool
	InfrastructurePassed bool
	UpgradeRan           bool
	UpgradePassed        bool
	InstallFailureMsg    string
	InfraFailureMsg      string
	UpgradeFailureMsg    string
}

type junitTestSuites struct {
	XMLName  xml.Name       `xml:"testsuites"`
	Tests    int            `xml:"tests,attr"`
	Failures int            `xml:"failures,attr"`
	Suites   []junitTestSuite `xml:"testsuite"`
}

type junitTestSuite struct {
	Name     string          `xml:"name,attr"`
	Tests    int             `xml:"tests,attr"`
	Failures int             `xml:"failures,attr"`
	Cases    []junitTestCase `xml:"testcase"`
}

type junitTestCase struct {
	Name      string        `xml:"name,attr"`
	ClassName string        `xml:"classname,attr"`
	Status    string        `xml:"status,attr"`
	Time      string        `xml:"time,attr"`
	Failure   *junitFailure `xml:"failure,omitempty"`
}

type junitFailure struct {
	Message string `xml:"message,attr"`
	Text    string `xml:",chardata"`
}

func WriteSippyJUnit(result SignalResult, outputPath string) error {
	var cases []junitTestCase
	var failures int

	cases = append(cases, makeTestCase(InstallTestName, result.InstallPassed, result.InstallFailureMsg))
	if !result.InstallPassed {
		failures++
	}

	cases = append(cases, makeTestCase(InfrastructureTestName, result.InfrastructurePassed, result.InfraFailureMsg))
	if !result.InfrastructurePassed {
		failures++
	}

	if result.UpgradeRan {
		cases = append(cases, makeTestCase(UpgradeTestName, result.UpgradePassed, result.UpgradeFailureMsg))
		if !result.UpgradePassed {
			failures++
		}
	}

	suites := junitTestSuites{
		Tests:    len(cases),
		Failures: failures,
		Suites: []junitTestSuite{{
			Name:     suiteName,
			Tests:    len(cases),
			Failures: failures,
			Cases:    cases,
		}},
	}

	data, err := xml.MarshalIndent(suites, "", "  ")
	if err != nil {
		return err
	}

	output := append([]byte(xml.Header), data...)
	return os.WriteFile(outputPath, output, 0644)
}

func makeTestCase(name string, passed bool, failureMsg string) junitTestCase {
	tc := junitTestCase{
		Name:      name,
		ClassName: suiteName,
		Status:    "passed",
		Time:      "0",
	}
	if !passed {
		tc.Status = "failed"
		tc.Failure = &junitFailure{
			Message: failureMsg,
			Text:    failureMsg,
		}
	}
	return tc
}
