package config

import (
	"math"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// Structural contract checks for the scheduled daily health report
// instruction file.
//
// The scheduled task is executed by the chai-bot runtime, not by code in this
// repository. These tests read the instruction file and assert the structure
// of the contract it states. They do not execute the scheduled task, do not
// observe a delivered summary, and therefore prove nothing about the task's
// actual output -- they guard the written contract against omissions and
// against samples that contradict it.

const dailyHealthReportDocPath = "../../.chai-bot/rosa_ci_daily_health_report.md"

// trendEmoji is the closed set of trend indicators the instruction file
// defines.
var trendEmoji = map[string]bool{
	":chart_with_upwards_trend:":   true,
	":chart_with_downwards_trend:": true,
	":left_right_arrow:":           true,
}

const (
	// trendUnavailable is the literal the instruction file requires when a
	// comparison window is incomplete.
	trendUnavailable = "trend unavailable"
	// zeroDenominator is the literal for a complete current window that held
	// no completed build.
	zeroDenominator = "N/A (0/0)"
	// dataUnavailable is the literal for an incomplete current window whose
	// retrieved subset held no completed build, where the totals are unknown.
	dataUnavailable = "N/A (data unavailable)"
)

var (
	countsRe = regexp.MustCompile(`\((\d+)/(\d+)[^)]*\)`)
	headerRe = regexp.MustCompile(`-- (\d+)%\*$`)
)

func loadDailyHealthReportDoc(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(dailyHealthReportDocPath)
	if err != nil {
		t.Fatalf("failed to read %s: %v", dailyHealthReportDocPath, err)
	}
	return string(data)
}

// sampleLine is one line of a fenced code block: a line the instruction file
// shows the scheduled task as an example to imitate. Prose is excluded so that
// rewording the surrounding text cannot fail these tests.
type sampleLine struct {
	number int // 1-indexed line number in the file
	block  int // index of the fenced block the line belongs to
	text   string
}

// loadSampleBlocks returns the fenced code blocks of the instruction file,
// each as its own slice of lines. Blocks are kept separate so that adjacency
// is only ever asserted between lines of the same block.
func loadSampleBlocks(t *testing.T) [][]sampleLine {
	t.Helper()
	var blocks [][]sampleLine
	var current []sampleLine
	inFence := false
	for i, raw := range strings.Split(loadDailyHealthReportDoc(t), "\n") {
		line := strings.TrimSpace(raw)
		if strings.HasPrefix(line, "```") {
			if inFence {
				blocks = append(blocks, current)
				current = nil
			}
			inFence = !inFence
			continue
		}
		if inFence {
			current = append(current, sampleLine{number: i + 1, block: len(blocks), text: line})
		}
	}
	if inFence {
		t.Fatalf("%s has an unterminated fenced block", dailyHealthReportDocPath)
	}
	if len(blocks) == 0 {
		t.Fatalf("%s contains no fenced sample blocks", dailyHealthReportDocPath)
	}
	return blocks
}

// trendField returns the trend value of a rollup sample line: the text after
// the first " -- " that follows the parenthesised rate/counts. Taking the
// first rather than the last separator matters because the instruction file
// allows a further " -- {brief inline note}" after the trend.
func trendField(line string) (string, bool) {
	rest := line
	if close := strings.Index(line, ")"); close >= 0 {
		rest = line[close+1:]
	}
	idx := strings.Index(rest, " -- ")
	if idx < 0 {
		return "", false
	}
	field := strings.TrimSpace(rest[idx+len(" -- "):])
	if next := strings.Index(field, " -- "); next >= 0 {
		field = strings.TrimSpace(field[:next])
	}
	return field, field != ""
}

func isRollup(line string) bool {
	return strings.HasPrefix(line, "*Gating:*") || strings.HasPrefix(line, "*Non-gating:*")
}

func isHeader(line string) bool {
	return strings.HasPrefix(line, "*ROSA CI Daily Health")
}

// TestDailyHealthReportHeadlineBlock asserts that in every fenced summary
// sample -- the format template and each worked example -- the header line is
// immediately followed, within that same block, by the Gating and Non-gating
// rollup lines.
func TestDailyHealthReportHeadlineBlock(t *testing.T) {
	headers, templates := 0, 0
	for _, block := range loadSampleBlocks(t) {
		for i, sample := range block {
			if !isHeader(sample.text) {
				continue
			}
			headers++
			if strings.Contains(sample.text, "{DATE}") {
				templates++
			}
			if i+2 >= len(block) {
				t.Errorf("line %d: header is not followed by two rollup lines in the same block", sample.number)
				continue
			}
			gating, nonGating := block[i+1], block[i+2]
			if !strings.HasPrefix(gating.text, "*Gating:*") {
				t.Errorf("line %d: header must be followed by *Gating:*, got %q", gating.number, gating.text)
			}
			if !strings.HasPrefix(nonGating.text, "*Non-gating:*") {
				t.Errorf("line %d: *Gating:* must be followed by *Non-gating:*, got %q", nonGating.number, nonGating.text)
			}
		}
	}
	if templates != 1 {
		t.Errorf("expected exactly one {DATE} format template summary, found %d", templates)
	}
	if headers < templates+1 {
		t.Errorf("expected the format template and at least one worked example, found %d summary headers", headers)
	}
}

// TestDailyHealthReportRollupTrendField asserts every rollup line in every
// sample carries a trend value that is a defined trend emoji, a template
// placeholder, or the literal unavailable fallback. Anything else is a value
// the instruction file never defines.
func TestDailyHealthReportRollupTrendField(t *testing.T) {
	for _, block := range loadSampleBlocks(t) {
		for _, sample := range block {
			if !isRollup(sample.text) {
				continue
			}
			field, ok := trendField(sample.text)
			if !ok {
				t.Errorf("line %d: rollup line has no trend field (ASCII %q separator required): %q",
					sample.number, " -- ", sample.text)
				continue
			}
			if strings.HasPrefix(field, "{") || field == trendUnavailable || trendEmoji[field] {
				continue
			}
			t.Errorf("line %d: trend field must be a placeholder, a defined trend emoji or the literal %q, got %q",
				sample.number, trendUnavailable, field)
		}
	}
}

// TestDailyHealthReportSamplesObeyDegradedValues asserts the worked examples
// practise the degradation rules rather than only stating them: no sample
// reports a zero denominator as 0%, an N/A rollup never carries a measured
// trend, and both N/A forms appear in at least one example.
func TestDailyHealthReportSamplesObeyDegradedValues(t *testing.T) {
	var zeroRollups, unknownRollups int
	for _, block := range loadSampleBlocks(t) {
		for _, sample := range block {
			if !isRollup(sample.text) {
				continue
			}
			if strings.Contains(sample.text, "0% (0/0)") {
				t.Errorf("line %d: a zero denominator must render as %q, never as 0%%: %q",
					sample.number, zeroDenominator, sample.text)
			}
			switch {
			case strings.Contains(sample.text, dataUnavailable):
				unknownRollups++
			case strings.Contains(sample.text, zeroDenominator):
				zeroRollups++
			default:
				continue
			}
			if field, _ := trendField(sample.text); field != trendUnavailable {
				t.Errorf("line %d: an N/A rollup has no measurable trend and must show %q, got %q",
					sample.number, trendUnavailable, field)
			}
		}
	}
	if zeroRollups == 0 {
		t.Errorf("expected a worked example showing a %q rollup", zeroDenominator)
	}
	if unknownRollups == 0 {
		t.Errorf("expected a worked example showing a %q rollup", dataUnavailable)
	}
}

// TestDailyHealthReportUnknownDataDistinction asserts the instruction file
// keeps the two N/A forms distinct: a confirmed-empty complete current window
// versus unknown totals from an incomplete one. Collapsing them would let a
// run with no retrieved history claim that every job ran nothing.
func TestDailyHealthReportUnknownDataDistinction(t *testing.T) {
	doc := loadDailyHealthReportDoc(t)
	for _, needle := range []string{
		"`" + zeroDenominator + "`",
		"`" + dataUnavailable + "`",
		", incomplete)",
		"Never print `0/0` for an incomplete window",
	} {
		if !strings.Contains(doc, needle) {
			t.Errorf("instruction file must distinguish confirmed-empty from unknown totals (missing %q)", needle)
		}
	}
	// An incomplete window must never be rendered with 0/0, in prose or sample.
	if strings.Contains(doc, "0/0, incomplete") {
		t.Error("an incomplete window must use " + dataUnavailable + ", not a 0/0 count")
	}
}

// TestDailyHealthReportMissingTrendExample asserts the file keeps a worked
// example for the incomplete-comparison-window case, that the example carries
// both rollup lines, and that both use the literal fallback.
func TestDailyHealthReportMissingTrendExample(t *testing.T) {
	const heading = "*Comparison history capped, truncated or missing:*"
	doc := loadDailyHealthReportDoc(t)
	start := strings.Index(doc, heading)
	if start < 0 {
		t.Fatalf("instruction file must keep a worked example headed %q", heading)
	}
	headingLine := strings.Count(doc[:start], "\n") + 1

	var example []sampleLine
	for _, block := range loadSampleBlocks(t) {
		if len(block) > 0 && block[0].number > headingLine {
			example = block
			break
		}
	}
	if example == nil {
		t.Fatalf("no fenced sample block follows %q", heading)
	}
	seen := map[string]bool{}
	for _, sample := range example {
		for _, label := range []string{"*Gating:*", "*Non-gating:*"} {
			if !strings.HasPrefix(sample.text, label) {
				continue
			}
			seen[label] = true
			if field, _ := trendField(sample.text); field != trendUnavailable {
				t.Errorf("line %d: missing-history example %s must show %q, got %q",
					sample.number, label, trendUnavailable, field)
			}
		}
	}
	for _, label := range []string{"*Gating:*", "*Non-gating:*"} {
		if !seen[label] {
			t.Errorf("missing-history example must contain a %s rollup line", label)
		}
	}
}

// TestDailyHealthReportExampleArithmetic asserts each worked example's header
// rate equals the weighted rate of its own two rollup lines, so the samples
// cannot teach a figure that contradicts the aggregation rule in step 3.
func TestDailyHealthReportExampleArithmetic(t *testing.T) {
	checked := 0
	for _, block := range loadSampleBlocks(t) {
		for i, sample := range block {
			if !isHeader(sample.text) || strings.Contains(sample.text, "{") {
				continue
			}
			want := headerRe.FindStringSubmatch(sample.text)
			if want == nil {
				continue // an N/A header carries no rate to reconcile
			}
			var pass, total int
			for _, roll := range block[i+1:] {
				if !isRollup(roll.text) {
					break
				}
				m := countsRe.FindStringSubmatch(roll.text)
				if m == nil {
					continue
				}
				p, _ := strconv.Atoi(m[1])
				tt, _ := strconv.Atoi(m[2])
				pass += p
				total += tt
			}
			if total == 0 {
				t.Errorf("line %d: header shows a rate but its rollups carry no counts", sample.number)
				continue
			}
			got := int(math.Round(float64(pass) / float64(total) * 100))
			expected, _ := strconv.Atoi(want[1])
			if got != expected {
				t.Errorf("line %d: header rate %d%% does not match its rollups %d/%d = %d%%",
					sample.number, expected, pass, total, got)
			}
			checked++
		}
	}
	if checked == 0 {
		t.Error("expected at least one worked example with a numeric header rate")
	}
}

// TestDailyHealthReportDefinesDegradedValues asserts the instruction file
// defines a value for every field that can be unfillable, so the task can
// degrade the field rather than omit the line.
func TestDailyHealthReportDefinesDegradedValues(t *testing.T) {
	doc := loadDailyHealthReportDoc(t)
	required := []struct{ needle, why string }{
		{"`" + trendUnavailable + "`", "the literal trend fallback value"},
		{"`" + zeroDenominator + "`", "the confirmed-empty value"},
		{"never `0%`", "the rule against reporting a zero denominator as 0%"},
		{"partial data ({N} jobs unavailable)", "the partial-coverage flag that keeps unretrieved jobs out of the rates"},
		{"Pre-delivery structural checklist", "the pre-delivery structural check"},
		{"Window completeness", "when a window counts as complete"},
		{"each window holds at least one completed build", "that a trend needs two comparable rates, not merely two complete windows"},
		{", incomplete", "the qualifier for a rate drawn from an incomplete current window"},
	}
	for _, r := range required {
		if !strings.Contains(doc, r.needle) {
			t.Errorf("instruction file must define %s (missing %q)", r.why, r.needle)
		}
	}
	if strings.Contains(doc, "trend n/a") {
		t.Errorf("the trend fallback literal is %q; %q must not survive anywhere", trendUnavailable, "trend n/a")
	}
}

// TestDailyHealthReportBrevityFloor asserts the brevity guidance keeps a floor
// under a delivered report. The report may be withheld entirely, but a
// delivered one may not be shortened below the headline block.
func TestDailyHealthReportBrevityFloor(t *testing.T) {
	doc := loadDailyHealthReportDoc(t)
	if strings.Contains(doc, "a one-liner is fine") {
		t.Error("brevity guidance must not permit a one-liner summary; it conflicts with the mandatory headline block")
	}
	for _, needle := range []string{
		"never between the required summary and an abbreviated one",
		"headline block is never dropped",
	} {
		if !strings.Contains(doc, needle) {
			t.Errorf("brevity guidance must keep the floor under a delivered report (missing %q)", needle)
		}
	}
}

// TestDailyHealthReportOverflowPreservesAnalysis asserts the overflow rule,
// which may drop category lines from the summary, does not also drop those
// categories' mandatory failure analysis: the threading gate must follow the
// computed health rather than the emitted lines.
func TestDailyHealthReportOverflowPreservesAnalysis(t *testing.T) {
	doc := loadDailyHealthReportDoc(t)
	for _, needle := range []string{
		"Dropping a category *line* never drops that category's step 5 analysis",
		"did this run **compute** any :red_circle: or :large_yellow_circle: category",
	} {
		if !strings.Contains(doc, needle) {
			t.Errorf("overflow must not suppress mandatory failure analysis (missing %q)", needle)
		}
	}
}
