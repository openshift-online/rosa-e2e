package config

import (
	"fmt"
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

	// breakdownHeading is the section that states the complete category
	// breakdown contract.
	breakdownHeading = "## Additional complete category breakdown"

	// State markers a breakdown row uses instead of a rate. They are mutually
	// exclusive and must stay distinguishable: collapsing them would let an
	// unretrieved window read as a category that genuinely did not run.
	noRunsMarker        = "`no runs`"
	partialMarker       = "`partial`"
	unavailableMarker   = "`unavailable`"
	informingOnlyMarker = "`informing-only`"

	// detailsWithheld is the fixed suffix a yellow or red summary line carries
	// in place of failure detail, which belongs in the step 5 thread.
	detailsWithheld = "-- details withheld"
	// informingOnlySuffix marks a summary line whose figures are informational
	// because the category has no non-informing job.
	informingOnlySuffix = "-- informing-only"
	// informingOnlyTally is the count-only informing figure a mixed category
	// carries beside its non-informing rate.
	informingOnlyTally = "· informing-only "
)

// categoryRank orders the summary's category groups. The emoji sequence is the
// grouping -- the instruction file forbids group headings -- so the rank of
// each health indicator is what a group-order assertion has to work from.
var categoryRank = map[string]int{
	":large_green_circle:":  0,
	":large_yellow_circle:": 1,
	":red_circle:":          2,
	":white_circle:":        3,
}

var (
	// The jobs link is a real prow_filter URL in a delivered report and the
	// literal placeholder in the format template, so the pattern must accept
	// both. Anchoring on the placeholder alone would let any sample written
	// the way the bot actually renders it escape every check below.
	categoryLineRe = regexp.MustCompile(`^(:[a-z_]+:) \*[^*]+:\* (.*) \(<[^|]*\|jobs>\)$`)
	// rateSpanRe covers every rate form step 3 defines, plus the template
	// placeholder, so a residual check can strip the rate and see what is left.
	rateSpanRe = regexp.MustCompile(
		`^(?:\{rate\}% \(\{pass\}/\{total\}\)|\d+% \(\d+/\d+(?:, incomplete)?\)|N/A \(0/0\)|N/A \(data unavailable\))`)
	ratePrefixRe = regexp.MustCompile(`^(\d+)% \((\d+)/(\d+)(, incomplete)?\)`)
	tallyRe      = regexp.MustCompile(`· informing-only (?:(\d+)/(\d+)|\{ipass\}/\{itotal\})`)
	informingRe  = regexp.MustCompile(`^\*Informing:\* (\d+)/(\d+) passing`)
	trendTokenRe = regexp.MustCompile(`^(?::[a-z_]+:|trend unavailable|\{trend\})`)
	// charCapRe catches any reintroduced length cap on the summary, however
	// it is worded. The removed rule traded category lines against 1200
	// characters; an exact-literal guard would miss a reworded revival.
	charCapRe = regexp.MustCompile(`(?i)(\d[\d,]{2,} character|character (?:cap|limit)|summary limit)`)
)

// categorySample is one parsed category line of a fenced sample block.
type categorySample struct {
	line    sampleLine
	emoji   string
	body    string // everything between the category name and the jobs link
	printed int    // the percentage the line actually prints
	pass    int
	total   int
	counted bool // body opens with numeric counts, qualified or not
	rated   bool // ... and its current window is complete, so the rate is comparable
	// zeroDenom and unknown keep step 3's two N/A forms apart. A confirmed
	// empty window contributes a real zero to the sums; unknown totals
	// contribute nothing at all and must never be read as zero.
	zeroDenom bool
	unknown   bool
	marked    bool // carries the informing-only suffix
	iPass     int
	iTotal    int
	tally     bool
	residual  string // body text left over once every defined token is removed
}

// categorySamples returns the category lines of one fenced block, in the order
// the block lists them.
func categorySamples(t *testing.T, block []sampleLine) []categorySample {
	t.Helper()
	var out []categorySample
	for _, line := range block {
		m := categoryLineRe.FindStringSubmatch(line.text)
		if m == nil {
			continue
		}
		c := categorySample{line: line, emoji: m[1], body: m[2]}
		if r := ratePrefixRe.FindStringSubmatch(c.body); r != nil {
			c.printed = atoi(t, line, r[1])
			c.pass = atoi(t, line, r[2])
			c.total = atoi(t, line, r[3])
			c.counted = true
			// An incomplete window still contributes its retrieved counts
			// (step 3), but its rate is not a comparable measurement.
			c.rated = r[4] == ""
		}
		c.zeroDenom = strings.HasPrefix(c.body, zeroDenominator)
		c.unknown = strings.HasPrefix(c.body, dataUnavailable)
		c.marked = strings.Contains(c.body, informingOnlySuffix)
		if tt := tallyRe.FindStringSubmatch(c.body); tt != nil {
			c.tally = true
			if tt[1] != "" {
				c.iPass = atoi(t, line, tt[1])
				c.iTotal = atoi(t, line, tt[2])
			}
		}
		c.residual = residualBody(c.body)
		out = append(out, c)
	}
	return out
}

func atoi(t *testing.T, line sampleLine, s string) int {
	t.Helper()
	n, err := strconv.Atoi(s)
	if err != nil {
		t.Fatalf("line %d: unparsable number %q", line.number, s)
	}
	return n
}

// residualBody strips every token a category line is allowed to carry -- the
// rate span, the trend, the withheld-detail suffix, the informing-only suffix
// and the informing tally -- and returns whatever is left. Anything left over
// is text the instruction file never defines, which on a yellow or red line is
// exactly the leaked failure analysis the thread exists to hold. Checking the
// whole body rather than only the text after the suffix is what catches a note
// placed before it.
func residualBody(body string) string {
	rest := rateSpanRe.ReplaceAllString(body, "")
	rest = strings.TrimSpace(rest)
	rest = trendTokenRe.ReplaceAllString(rest, "")
	rest = tallyRe.ReplaceAllString(rest, "")
	rest = strings.Replace(rest, detailsWithheld, "", 1)
	rest = strings.Replace(rest, informingOnlySuffix, "", 1)
	return strings.TrimSpace(rest)
}

// rollupCounts sums the Gating and Non-gating counts of a block. bounded stops
// at the first non-rollup line, which is how the headline arithmetic check
// scopes itself to the rollups directly under one header.
func rollupCounts(t *testing.T, lines []sampleLine, bounded bool) (pass, total int, ok bool) {
	t.Helper()
	for _, line := range lines {
		if !isRollup(line.text) {
			if bounded {
				break
			}
			continue
		}
		m := countsRe.FindStringSubmatch(line.text)
		if m == nil {
			continue
		}
		pass += atoi(t, line, m[1])
		total += atoi(t, line, m[2])
		ok = true
	}
	return pass, total, ok
}

// indicatorFor maps a pass rate to the health-indicator rank step 3 defines:
// green >= 80%, yellow >= 40%, red below that. An informing-only category is
// :white_circle: regardless, so callers exempt that rank.
func indicatorFor(rate int) int {
	switch {
	case rate >= 80:
		return categoryRank[":large_green_circle:"]
	case rate >= 40:
		return categoryRank[":large_yellow_circle:"]
	default:
		return categoryRank[":red_circle:"]
	}
}

func indicatorName(rank int) string {
	for emoji, r := range categoryRank {
		if r == rank {
			return emoji
		}
	}
	return "unknown"
}

// isElided reports whether a block omits category lines with a bare "..."
// continuation, which makes it unusable for whole-list reconciliation.
func isElided(block []sampleLine) bool {
	for _, line := range block {
		if line.text == "..." {
			return true
		}
	}
	return false
}

var (
	countsRe           = regexp.MustCompile(`\((\d+)/(\d+)[^)]*\)`)
	headerRe           = regexp.MustCompile(`-- (\d+)%\*$`)
	breakdownPointerRe = regexp.MustCompile(`full breakdown in thread \((\d+) categories\)`)
)

// footerClauseOrder is the canonical order of the summary footer's clauses, as
// rendered. Needles are chosen to match both the format template and the
// worked examples, singular and plural alike. No sample carries a drift clause
// today; its entry guards the order of one that does.
var footerClauseOrder = []string{
	"skipped (no runs)",
	"partial data (",
	"full breakdown in thread (",
	"|Sippy>",
	"|Prow>",
	"|Dashboard>",
	"Registry drift:",
}

// footerClauseLabels names the same clauses in the same order as the prose
// rule states them, so the rendered order and the stated order cannot drift
// apart unnoticed.
var footerClauseLabels = []string{
	"skipped-category count",
	"partial data",
	"complete-breakdown pointer",
	"Sippy",
	"Prow",
	"Dashboard",
	"`Registry drift:`",
}

// footerLinks must appear in every footer, whatever else the run dropped.
var footerLinks = []string{"|Sippy>", "|Prow>", "|Dashboard>"}

func loadDailyHealthReportDoc(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(dailyHealthReportDocPath)
	if err != nil {
		t.Fatalf("failed to read %s: %v", dailyHealthReportDocPath, err)
	}
	return string(data)
}

// docSection returns the text of one top-level section, from its heading to
// the next one. Scoping a needle to its section keeps a rule from passing the
// test after it migrates somewhere it no longer governs. The heading is
// anchored to a line start so a mention of it in prose cannot retarget the
// slice.
func docSection(t *testing.T, heading string) string {
	t.Helper()
	doc := loadDailyHealthReportDoc(t)
	start := strings.Index(doc, "\n"+heading+"\n")
	if start < 0 {
		t.Fatalf("instruction file must keep a section headed %q", heading)
	}
	section := doc[start+1:]
	if next := strings.Index(section[len(heading):], "\n## "); next >= 0 {
		section = section[:len(heading)+next]
	}
	return section
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
		// The coverage check reconciles the same two forms, and once said
		// "nothing retrieved ... contributes zero" -- which is the claim
		// step 3 reserves for a confirmed-empty window.
		"contributes a real `0/0`. It is measured, and zero is the measurement",
		"contribute neither a count nor a zero",
		"Treating unknown as zero would assert the category ran nothing",
		"check 3 cannot be completed for that run: report it as inconclusive",
		"coverage check inconclusive:",
		"every **known** informing tally contributes to it whatever rate form sits beside it",
	} {
		if !strings.Contains(doc, needle) {
			t.Errorf("instruction file must distinguish confirmed-empty from unknown totals (missing %q)", needle)
		}
	}
	if strings.Contains(doc, "nothing retrieved, no completed build, or informing jobs only — contributes zero") {
		t.Error("the coverage check must not fold unknown totals in with a confirmed-empty window as a zero contributor")
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
		"Brevity also never costs a category its figures",
	} {
		if !strings.Contains(doc, needle) {
			t.Errorf("brevity guidance must keep the floor under a delivered report (missing %q)", needle)
		}
	}
}

// TestDailyHealthReportSummaryKeepsEveryCategoryLine asserts the summary
// carries the whole grouped category list. The superseded contract traded the
// list against a 1200-character cap and relocated every line into the thread,
// which made the top-level scoreboard disappear on exactly the registries that
// needed it most.
func TestDailyHealthReportSummaryKeepsEveryCategoryLine(t *testing.T) {
	doc := loadDailyHealthReportDoc(t)
	for _, needle := range []string{
		"The complete category list stays in the summary.",
		"There is no overflow rule",
		"not truncated to the lines that fit",
		"not relocated into the thread",
	} {
		if !strings.Contains(doc, needle) {
			t.Errorf("the summary must keep every category line (missing %q)", needle)
		}
	}
	for _, forbidden := range []string{
		"drop the per-category list **as a whole**",
		"the summary carries either every category line or none",
		"list only the worst-performing categories that fit",
		"append a `+{N} more categories` clause",
	} {
		if strings.Contains(doc, forbidden) {
			t.Errorf("the superseded overflow behaviour must not survive (found %q)", forbidden)
		}
	}
	// A reworded length cap revives the same defect, so match the shape of
	// one rather than the exact sentence that used to state it.
	for _, m := range charCapRe.FindAllString(doc, -1) {
		t.Errorf("a summary length cap must not be reintroduced (found %q)", m)
	}
}

// TestDailyHealthReportStatesSummaryRules asserts the summary rules the bot
// acts on are actually written down. The samples pin what a correct report
// looks like, but the bot follows the prose: a rule that exists only as an
// example is a rule the instruction file does not state.
func TestDailyHealthReportStatesSummaryRules(t *testing.T) {
	doc := loadDailyHealthReportDoc(t)
	required := []struct{ needle, why string }{
		{"**Group order: green, then yellow, then red, then informing-only.**", "the group order"},
		{"within each group by pass rate descending", "the sort inside a group"},
		{"emit no group heading", "that the emoji sequence alone marks the groups"},
		{"**One category per line.**", "one line per category"},
		{"This is a fixed literal, not a summary", "that the withheld suffix is not a place for a note"},
		{"never put an error message, a failing job or test name, a date, a count or any other failure analysis on a summary line",
			"what may not appear on a summary line"},
		{"exclude them from the overall rate, from the gating and non-gating rates, and from per-category pass rates and health indicators",
			"that informing jobs are excluded from every aggregate"},
		{"append the count-only tally `" + informingOnlyTally + "{ipass}/{itotal}`", "the mixed-category tally"},
		{"never folded into it", "that the tally stays out of the rate"},
		{"**Rate population**", "the population every availability rule is judged over"},
		{"Aggregate two separate weighted pass rates across all **non-informing** jobs with data",
			"that the two rollups are computed over non-informing jobs only"},
		{"An informing job never enters either rate", "that no gating override can pull an informing job into a rollup"},
		{"`{overall_rate}` is the weighted pass rate across all **non-informing** jobs with data",
			"that the overall rate excludes informing jobs"},
	}
	for _, r := range required {
		if !strings.Contains(doc, r.needle) {
			t.Errorf("the summary rules must state %s (missing %q)", r.why, r.needle)
		}
	}
	// The pre-delivery checklist is what the bot runs immediately before
	// composing, so the same rules must survive there too. Bound the slice to
	// the checklist itself: running it to the end of the document would let
	// the scenario examples satisfy these needles after the checklist item
	// stating them had been deleted.
	start := strings.Index(doc, "Pre-delivery structural checklist")
	if start < 0 {
		t.Fatal("instruction file must keep the pre-delivery structural checklist")
	}
	end := strings.Index(doc[start:], "Never satisfy a failing item")
	if end < 0 {
		t.Fatal("the pre-delivery checklist must end with the rule against deleting a required line")
	}
	checklist := doc[start : start+end]
	for _, needle := range []string{
		"grouped green, then yellow, then red, then informing-only",
		"Every yellow and red line ends its trend field with the exact suffix",
		detailsWithheld,
		informingOnlySuffix,
		informingOnlyTally,
	} {
		if !strings.Contains(checklist, needle) {
			t.Errorf("the pre-delivery checklist must enforce %q", needle)
		}
	}
}

// TestDailyHealthReportCategoryGroupOrder asserts every sample lists its
// category lines green, then yellow, then red, then informing-only, with pass
// rate descending inside a group. The emoji sequence is the only thing marking
// the groups -- the file forbids group headings -- so a sample in the wrong
// order teaches the grouping away.
func TestDailyHealthReportCategoryGroupOrder(t *testing.T) {
	grouped, compared := 0, 0
	for _, block := range loadSampleBlocks(t) {
		samples := categorySamples(t, block)
		if len(samples) < 2 {
			continue
		}
		grouped++
		for _, c := range samples {
			// A line whose percentage disagrees with its own counts teaches a
			// figure the bot would never compute, and the health emoji beside
			// it is then unverifiable too.
			if c.counted && c.total > 0 {
				if want := int(math.Round(float64(c.pass) / float64(c.total) * 100)); want != c.printed {
					t.Errorf("line %d: prints %d%% but its counts %d/%d are %d%%",
						c.line.number, c.printed, c.pass, c.total, want)
				}
			}
			if rank, ok := categoryRank[c.emoji]; ok && c.rated {
				if got := indicatorFor(c.printed); got != rank && rank != categoryRank[":white_circle:"] {
					t.Errorf("line %d: %d%% is a %s category by the step 3 thresholds, not %s",
						c.line.number, c.printed, indicatorName(got), c.emoji)
				}
			}
		}
		for i := 1; i < len(samples); i++ {
			prev, cur := samples[i-1], samples[i]
			prevRank, ok := categoryRank[prev.emoji]
			if !ok {
				t.Errorf("line %d: %q is not a defined health indicator", prev.line.number, prev.emoji)
				continue
			}
			curRank, ok := categoryRank[cur.emoji]
			if !ok {
				t.Errorf("line %d: %q is not a defined health indicator", cur.line.number, cur.emoji)
				continue
			}
			if curRank < prevRank {
				t.Errorf("line %d: %s follows %s; groups run green, yellow, red, informing-only",
					cur.line.number, cur.emoji, prev.emoji)
				continue
			}
			if curRank == prevRank && prev.rated && cur.rated {
				compared++
				if cur.pass*prev.total > prev.pass*cur.total {
					t.Errorf("line %d: %d/%d sorts above %d/%d inside one group; sort by pass rate descending",
						cur.line.number, cur.pass, cur.total, prev.pass, prev.total)
				}
			}
		}
	}
	if grouped == 0 {
		t.Error("expected at least one sample listing several category lines in group order")
	}
	if compared == 0 {
		t.Error("no sample places two measured categories in one group, so the rate-descending sort is never exercised")
	}
}

// TestDailyHealthReportWithheldDetail asserts yellow and red summary lines
// carry the fixed withheld-detail suffix and nothing else. The superseded
// contract put a free-text failure note here, which leaked the analysis the
// thread exists to hold.
func TestDailyHealthReportWithheldDetail(t *testing.T) {
	doc := loadDailyHealthReportDoc(t)
	if !strings.Contains(doc, "append the exact suffix `"+detailsWithheld+"`") {
		t.Errorf("the file must fix the yellow/red suffix as %q", detailsWithheld)
	}
	for _, forbidden := range []string{
		"add a **short** inline note after the trend field",
		"Keep notes under 40 characters",
		"-- AMD64 & E2E at 40%",
		"-- stale since Jun 17",
	} {
		if strings.Contains(doc, forbidden) {
			t.Errorf("the superseded inline failure note must not survive (found %q)", forbidden)
		}
	}

	var yellow, red int
	for _, block := range loadSampleBlocks(t) {
		for _, c := range categorySamples(t, block) {
			withheld := strings.Contains(c.body, detailsWithheld)
			// Residual text is anything the file never defines for a category
			// line, wherever on the line it sits -- before the suffix as well
			// as after it.
			if c.residual != "" {
				t.Errorf("line %d: %q is undefined text on a summary line; failure detail belongs in the step 5 thread",
					c.line.number, c.residual)
			}
			switch c.emoji {
			case ":large_yellow_circle:":
				if c.rated {
					yellow++
				}
			case ":red_circle:":
				if c.rated {
					red++
				}
			default:
				if withheld {
					t.Errorf("line %d: only yellow and red lines withhold detail, got %s", c.line.number, c.emoji)
				}
				continue
			}
			if !withheld {
				t.Errorf("line %d: a %s line must carry %q, got %q", c.line.number, c.emoji, detailsWithheld, c.body)
			}
		}
	}
	if yellow == 0 || red == 0 {
		t.Errorf("expected worked yellow and red category lines with real figures, found %d yellow and %d red", yellow, red)
	}
}

// TestDailyHealthReportInformingOnlyRows asserts the hyphenated marker is the
// one spelling, that an informing-only line is the white-circle final group,
// and that a mixed category keeps its non-informing rate with the informing
// figures beside it rather than inside it.
func TestDailyHealthReportInformingOnlyRows(t *testing.T) {
	doc := loadDailyHealthReportDoc(t)
	if m := regexp.MustCompile(`(?i)\binforming only\b`).FindString(doc); m != "" {
		t.Errorf("the marker is spelled `informing-only`; the unhyphenated form must not survive (found %q)", m)
	}
	for _, needle := range []string{
		"Informing figures (informational only, never aggregated)",
		"its rate, counts, trend and health indicator stay exactly as computed above, over its non-informing jobs only",
		"print them as **informational** figures",
		"excluded from the overall, gating and non-gating rollups",
		"append the exact suffix `" + informingOnlySuffix + "`",
	} {
		if !strings.Contains(doc, needle) {
			t.Errorf("informing handling must be stated (missing %q)", needle)
		}
	}

	var informingOnly, mixed int
	for _, block := range loadSampleBlocks(t) {
		for _, c := range categorySamples(t, block) {
			marked := c.marked
			// The suffix implies the white circle, but not the reverse: the
			// final group also holds a mixed category whose non-informing
			// jobs returned no data, which takes an N/A release-signal rate
			// and its informing tally with no suffix at all.
			switch {
			case marked && c.emoji != ":white_circle:":
				t.Errorf("line %d: %q implies %s, got %s",
					c.line.number, informingOnlySuffix, ":white_circle:", c.emoji)
			case !marked && c.emoji == ":white_circle:":
				if c.counted {
					t.Errorf("line %d: a measured category takes a green, yellow or red indicator, not %s without %q",
						c.line.number, ":white_circle:", informingOnlySuffix)
				} else if !c.tally {
					t.Errorf("line %d: %s without %q is only the rate-less mixed row, which carries an informing tally",
						c.line.number, ":white_circle:", informingOnlySuffix)
				}
			}
			if marked {
				if c.rated {
					informingOnly++
				}
				// marked implies white above, so this covers the
				// informing-only row wherever its emoji lands.
				if c.tally {
					t.Errorf("line %d: an informing-only row reports that population as its own rate; it carries no separate tally",
						c.line.number)
				}
			}
			if c.tally && c.rated && c.iTotal > 0 {
				mixed++
			}
			if strings.Contains(c.body, "informing") && !marked && !c.tally {
				t.Errorf("line %d: unrecognised informing figure %q", c.line.number, c.body)
			}
		}
	}
	if informingOnly == 0 || mixed == 0 {
		t.Errorf("expected worked informing-only and mixed category lines with real figures, found %d and %d",
			informingOnly, mixed)
	}
}

// TestDailyHealthReportInformingOnlyIsNotFailingHealth asserts every gate that
// used to read "any category below 80%" is expressed in health-indicator terms
// instead. Giving an informing-only category a visible informational rate puts
// it inside a numeric threshold, where a 50% project-validation category would
// block the all-green path and -- worse -- trip the pre-send check demanding a
// failure-analysis reply that the threading gate forbids anyone to write.
func TestDailyHealthReportInformingOnlyIsNotFailingHealth(t *testing.T) {
	doc := loadDailyHealthReportDoc(t)
	required := []struct{ needle, why string }{
		{"**If no category is :large_yellow_circle: or :red_circle:**", "the all-green branch of step 4"},
		{"- Before sending: if any category is :large_yellow_circle: or :red_circle:", "the pre-send failure-analysis check"},
		{"On the all-green path (no :large_yellow_circle: or :red_circle: category)", "the all-green constraint"},
		{"Analyze the 5 worst :large_yellow_circle:/:red_circle: categories first", "the step 5 scope cap"},
		{"an informing-only category never qualifies", "the step 7 failure scope"},
		{"Informing figures never change which categories are selected for failure analysis",
			"that informational figures never widen the analysed set"},
		{"its informational rate is not release-signal health", "why an informing-only category cannot fail the all-green test"},
		{"health: \"informing-only\"", "the artifact convention that keeps the informational rate out of step 7's ranking"},
	}
	for _, r := range required {
		if !strings.Contains(doc, r.needle) {
			t.Errorf("an informing-only category must not read as failing health in %s (missing %q)", r.why, r.needle)
		}
	}
}

// TestDailyHealthReportSampleCategoriesReconcile asserts a complete worked
// summary adds up: its non-informing category counts equal the two rollups,
// and its informing figures equal the Informing rollup. This is what keeps a
// sample from teaching that informing builds leak into the aggregate rates.
func TestDailyHealthReportSampleCategoriesReconcile(t *testing.T) {
	checked := 0
	for _, block := range loadSampleBlocks(t) {
		if isElided(block) {
			continue // the block hides category lines, so its rows cannot sum
		}
		problems, compared := reconcileBlock(t, block)
		for _, p := range problems {
			t.Error(p)
		}
		if compared {
			checked++
		}
	}
	if checked == 0 {
		t.Error("expected a worked example whose complete category list reconciles with its rollups")
	}
}

// blockFigures is one fenced sample block's own arithmetic, gathered once so
// the release-signal and informing reconciliations can be judged separately.
// Keeping them apart is the point: an unknown release-signal total says
// nothing about an informing tally printed on the same row.
type blockFigures struct {
	samples               []categorySample
	reconcilable          int // rows contributing known release-signal figures
	unknown               int // rows whose release-signal totals are unknown
	catPass, catTotal     int
	tallyPass, tallyTotal int
	rollPass, rollTotal   int
	haveRollup            bool
	infPass, infTotal     int
	haveInforming         bool
}

func figuresOf(t *testing.T, block []sampleLine) blockFigures {
	t.Helper()
	f := blockFigures{samples: categorySamples(t, block)}
	f.rollPass, f.rollTotal, f.haveRollup = rollupCounts(t, block, false)
	for _, line := range block {
		if m := informingRe.FindStringSubmatch(line.text); m != nil {
			f.infPass = atoi(t, line, m[1])
			f.infTotal = atoi(t, line, m[2])
			f.haveInforming = true
		}
	}
	for _, c := range f.samples {
		// A known informing tally is known whatever rate form sits beside it.
		// Collecting it before the unknown branch is what keeps an
		// unretrieved release-signal window from hiding an informing error.
		f.tallyPass += c.iPass
		f.tallyTotal += c.iTotal
		switch {
		case c.unknown:
			// N/A (data unavailable) means the totals are not known. Counting
			// it as zero would assert the category ran nothing, which is
			// exactly the claim step 3 reserves for N/A (0/0).
			f.unknown++
		case c.marked:
			// An informing-only row's rate is its informing population.
			f.reconcilable++
			f.tallyPass += c.pass
			f.tallyTotal += c.total
		case c.counted || c.zeroDenom:
			// A zero-denominator row contributes a real 0/0 here, which is
			// what the coverage check calls a zero contributor.
			f.reconcilable++
			f.catPass += c.pass
			f.catTotal += c.total
		}
	}
	return f
}

// reconcileBlock checks one block, returning what did not reconcile and
// whether the release-signal sums were actually compared -- the latter is what
// the caller's coverage floor counts. Findings are returned rather than
// reported so the behaviour can be asserted directly. The informing rollup is
// checked independently, so it still runs on a block whose release-signal
// comparison is inconclusive.
func reconcileBlock(t *testing.T, block []sampleLine) (problems []string, compared bool) {
	t.Helper()
	f := figuresOf(t, block)
	if len(f.samples) == 0 || !f.haveRollup {
		return nil, false // a headline fragment carries no category rows to sum
	}
	at := f.samples[0].line.number

	// Informing reconciliation uses known tallies only and therefore stands
	// whatever the release-signal rows could not tell us.
	if f.tallyTotal > 0 && !f.haveInforming {
		problems = append(problems, fmt.Sprintf(
			"line %d: block shows informing figures (%d/%d) but no *Informing:* rollup",
			at, f.tallyPass, f.tallyTotal))
	}
	if f.haveInforming && (f.tallyPass != f.infPass || f.tallyTotal != f.infTotal) {
		problems = append(problems, fmt.Sprintf(
			"line %d: informing figures sum to %d/%d but the *Informing:* rollup shows %d/%d",
			at, f.tallyPass, f.tallyTotal, f.infPass, f.infTotal))
	}

	// Only the release-signal comparison needs every row's totals known.
	switch {
	case f.unknown > 0:
		return problems, false // inconclusive: unknown is not zero, so the sum would be short
	case f.reconcilable == 0:
		return problems, false // a template block has nothing to reconcile
	case f.reconcilable != len(f.samples):
		// Half-numeric blocks would reconcile against a partial sum and
		// quietly pass; that is the silent skip this test exists to avoid.
		return append(problems, fmt.Sprintf(
			"line %d: block mixes %d category lines carrying figures with %d that carry none; reconciliation cannot be checked",
			at, f.reconcilable, len(f.samples)-f.reconcilable)), false
	}
	if f.catPass != f.rollPass || f.catTotal != f.rollTotal {
		problems = append(problems, fmt.Sprintf(
			"line %d: category lines sum to %d/%d but the rollups carry %d/%d",
			at, f.catPass, f.catTotal, f.rollPass, f.rollTotal))
	}
	return problems, true
}

// syntheticBlock turns literal sample lines into the shape loadSampleBlocks
// produces, so a reconciliation case can be exercised without putting a
// deliberately wrong figure into the shipped instruction file.
func syntheticBlock(lines ...string) []sampleLine {
	block := make([]sampleLine, 0, len(lines))
	for i, text := range lines {
		block = append(block, sampleLine{number: i + 1, text: text})
	}
	return block
}

// TestReconcileBlockSeparatesUnknownFromZero pins the two behaviours the doc's
// coverage check distinguishes and that a doc-driven test cannot reach on its
// own: an unknown release-signal total must not be summed as zero, and it must
// not take the informing reconciliation down with it. The deliberately wrong
// rollups here are why these cases live in a synthetic block.
func TestReconcileBlockSeparatesUnknownFromZero(t *testing.T) {
	const (
		header  = "*ROSA CI Daily Health -- 2026-07-25 -- 90%*"
		gating  = "*Gating:* 90% (9/10) -- :left_right_arrow:"
		nonGate = "*Non-gating:* 90% (9/10) -- :left_right_arrow:"
		green   = ":large_green_circle: *A:* 90% (18/20) :left_right_arrow: (<prow_filter|jobs>)"
	)
	unavailableRow := ":white_circle: *B:* N/A (data unavailable) trend unavailable · informing-only 1/2 (<prow_filter|jobs>)"
	zeroRow := ":white_circle: *C:* N/A (0/0) trend unavailable · informing-only 1/2 (<prow_filter|jobs>)"

	for _, tc := range []struct {
		name      string
		block     []sampleLine
		wantErr   string // substring the failure must contain, "" for none
		wantCheck bool   // whether the release-signal sums were compared
	}{{
		name:      "unknown totals still reconcile a wrong informing rollup",
		block:     syntheticBlock(header, gating, nonGate, "*Informing:* 99/100 passing (excluded from pass rates)", green, unavailableRow),
		wantErr:   "informing figures sum to 1/2 but the *Informing:* rollup shows 99/100",
		wantCheck: false,
	}, {
		name:      "unknown totals leave a correct informing rollup alone",
		block:     syntheticBlock(header, gating, nonGate, "*Informing:* 1/2 passing (excluded from pass rates)", green, unavailableRow),
		wantErr:   "",
		wantCheck: false,
	}, {
		name:      "confirmed empty contributes a real zero and reconciles",
		block:     syntheticBlock(header, gating, nonGate, "*Informing:* 1/2 passing (excluded from pass rates)", green, zeroRow),
		wantErr:   "",
		wantCheck: true,
	}, {
		name:      "confirmed empty is not a licence to miscount the release signal",
		block:     syntheticBlock(header, "*Gating:* 90% (9/10) -- :left_right_arrow:", "*Non-gating:* 89% (8/9) -- :left_right_arrow:", "*Informing:* 1/2 passing (excluded from pass rates)", green, zeroRow),
		wantErr:   "category lines sum to 18/20 but the rollups carry 17/19",
		wantCheck: true,
	}} {
		t.Run(tc.name, func(t *testing.T) {
			problems, compared := reconcileBlock(t, tc.block)
			if compared != tc.wantCheck {
				t.Errorf("release-signal comparison ran = %v, want %v", compared, tc.wantCheck)
			}
			joined := strings.Join(problems, "\n")
			switch {
			case tc.wantErr == "" && len(problems) > 0:
				t.Errorf("expected the block to reconcile, got:\n%s", joined)
			case tc.wantErr != "" && !strings.Contains(joined, tc.wantErr):
				t.Errorf("expected a finding containing %q, got:\n%s", tc.wantErr, joined)
			case tc.wantErr != "" && len(problems) != 1:
				t.Errorf("expected exactly one finding, got %d:\n%s", len(problems), joined)
			}
		})
	}
}

// TestDailyHealthReportCompleteBreakdownContract asserts the file states the
// completeness contract an overflowed or no-run-trimmed summary depends on:
// one row per configured category, in the thread, from the same run, with the
// guarantee itself written down and the new reply type wired into every rule
// that enumerates reply types.
func TestDailyHealthReportCompleteBreakdownContract(t *testing.T) {
	doc := loadDailyHealthReportDoc(t)
	docScoped := []struct{ needle, why string }{
		{breakdownHeading, "the complete breakdown section"},
		{"exactly one line per configured category", "when the inline list is not already the complete view"},
		{"full breakdown in thread ({total} categories)", "the footer pointer to the complete breakdown"},
		{"Canonical footer clause order: skipped-category count · partial data · complete-breakdown pointer · Sippy · Prow · Dashboard · `Registry drift:`.", "the pointer's place in the canonical footer order"},
		{"absent from both the summary and the thread", "the checklist gate that no category falls through both"},
		{"out of both the summary and the thread", "the Constraints-level completeness guarantee"},
		{"it never satisfies this gate", "that the breakdown does not touch the failure-analysis gate"},
		{"at least one **failure-analysis** reply section", "that only a step 5 reply satisfies the pre-send delimiter check"},
		{"remediation, complete category breakdown or phase breakdown", "the new reply type in the single threading rule"},
		{"remediation or complete category breakdown", "the new reply type in the phase follow-up's ordering"},
		{"in the complete category breakdown reply instead", "where the summary's excluded no-run categories go"},
	}
	for _, r := range docScoped {
		if !strings.Contains(doc, r.needle) {
			t.Errorf("complete breakdown must define %s (missing %q)", r.why, r.needle)
		}
	}

	section := docSection(t, breakdownHeading)
	sectionScoped := []struct{ needle, why string }{
		{"one row per configured category in the pinned registry, in registry order", "one row per configured category, deterministically ordered"},
		{"every configured category `id` appears exactly once — no missing category, no duplicate row", "the coverage check against the configured category set"},
		{"the same run and the same reporting window as the summary", "that the breakdown reports the summary's own figures"},
		{"Do not issue a second history query", "that the breakdown adds no data collection"},
		{"Complete category breakdown ({n} of {replies})", "the labelled multi-reply fallback"},
		{"Never compose this reply, or its footer pointer, to defeat the scheduled silence", "that the breakdown cannot defeat scheduled silence"},
	}
	for _, r := range sectionScoped {
		if !strings.Contains(section, r.needle) {
			t.Errorf("complete breakdown section must define %s (missing %q)", r.why, r.needle)
		}
	}
}

// TestDailyHealthReportBreakdownRowFields asserts a breakdown row reuses the
// summary's own value vocabulary -- the four rate forms, the trend fallback,
// the markers that keep the kinds of missing data apart, and informing counts
// kept out of the rates -- instead of inventing a second set of conventions.
func TestDailyHealthReportBreakdownRowFields(t *testing.T) {
	section := docSection(t, breakdownHeading)
	required := []struct{ needle, why string }{
		{"{rate}% ({pass}/{total})", "the measured rate form with its counts"},
		{"`" + zeroDenominator + "`", "the confirmed-empty rate form"},
		{"`" + dataUnavailable + "`", "the unknown-totals rate form"},
		{", incomplete)", "the incomplete-window qualifier"},
		{"`" + trendUnavailable + "`", "the trend fallback"},
		{noRunsMarker, "the explicit no-run marker"},
		{partialMarker, "the explicit partial-coverage marker"},
		{unavailableMarker, "the nothing-retrieved marker"},
		{informingOnlyMarker, "the marker for a category with no rate population"},
		{"first match wins in this order", "a precedence that keeps the markers mutually exclusive"},
		{"the same population as its rate", "markers judged over the non-informing jobs, so a marker cannot contradict the rate beside it"},
		{"Never show an incomplete window as " + noRunsMarker, "that an incomplete window is never reported as no runs"},
		{"`informing-only {passing}/{total}`", "the count-only informing tally"},
		{"never folded into the rate", "that informing jobs stay out of the rates"},
		{"`prow_filter`", "the jobs link when the registry supplies one"},
	}
	for _, r := range required {
		if !strings.Contains(section, r.needle) {
			t.Errorf("breakdown row must carry %s (missing %q)", r.why, r.needle)
		}
	}
}

// TestDailyHealthReportIncompleteRateKeepsPartialMarker asserts the row
// markers stay consistent with step 3's Rate availability, which measures a
// qualified rate from whatever an incomplete window returned -- even when
// every window of the population is incomplete. Such a row is partial
// coverage over a measured rate: marking it unavailable would deny figures
// the report prints, and omitting its marker would hide that the coverage
// behind them is partial. A partial row may equally print
// N/A (data unavailable), when mixed coverage returned no completed build,
// so marker and rate form are not in one-to-one correspondence.
func TestDailyHealthReportIncompleteRateKeepsPartialMarker(t *testing.T) {
	section := docSection(t, breakdownHeading)
	required := []struct{ needle, why string }{
		{"A qualified measured rate `{rate}% ({pass}/{total}, incomplete)` is a " + partialMarker + " row, including when every window is incomplete",
			"that a measured incomplete rate is partial coverage, whatever share of the population is incomplete"},
		{"the marker never downgrades a measured rate to an `N/A` form",
			"that the marker never changes step 3's computed rate"},
		{"never omit the marker from a row whose rate carries the `, incomplete` qualifier",
			"that a qualified rate always keeps its coverage marker"},
		{"no non-informing window is complete **and** no completed build was retrieved",
			"the narrowed condition for the nothing-is-known marker"},
		{"Mixed coverage that returned no completed build is a " + partialMarker + " row too, and it prints the same `" + dataUnavailable + "` rate form as an " + unavailableMarker + " row",
			"that the unavailable rate form is shared by both markers, so mixed coverage stays partial"},
		{"different markers can accompany the same rate form, one marker can accompany different rate forms",
			"that marker and rate form are not in one-to-one correspondence, in both directions"},
		{"never upgrades an `N/A` form to a measured one",
			"that the marker never changes which rate form step 3 prints"},
		{"every non-informing window is complete and none held a completed build",
			"that no runs needs complete windows"},
		{"every non-informing window is complete and a rate is measured",
			"the only case that carries no marker"},
	}
	for _, r := range required {
		if !strings.Contains(section, r.needle) {
			t.Errorf("marker states must pin %s (missing %q)", r.why, r.needle)
		}
	}
	for _, forbidden := range []string{
		"omit the marker entirely for a category with a measured rate",
		"`unavailable` when the current window is incomplete for every non-informing job",
		"only case that pairs with",
		"only marker that pairs with",
		"pairs exclusively with",
	} {
		if strings.Contains(section, forbidden) {
			t.Errorf("a marker rule contradicting step 3's qualified measured rate must not survive (found %q)", forbidden)
		}
	}
}

// TestDailyHealthReportFooterClauseOrder asserts every sample footer carries
// the three links, orders its clauses canonically, and routes readers to the
// breakdown rather than counting what it leaves out. The samples are what the
// scheduled task imitates, so a sample contradicting the prose teaches the
// wrong footer. The prose rule is checked against the same order.
func TestDailyHealthReportFooterClauseOrder(t *testing.T) {
	assertOrder := func(what, in string, clauses []string, line int) {
		t.Helper()
		prevAt, prevClause := -1, ""
		for _, clause := range clauses {
			at := strings.Index(in, clause)
			if at < 0 {
				continue
			}
			if at < prevAt {
				t.Errorf("line %d: %s clause %q precedes %q; canonical order is %v",
					line, what, clause, prevClause, clauses)
			}
			prevAt, prevClause = at, clause
		}
	}

	doc := loadDailyHealthReportDoc(t)
	const canonical = "Canonical footer clause order:"
	start := strings.Index(doc, canonical)
	if start < 0 {
		t.Fatalf("instruction file must state the canonical footer clause order (missing %q)", canonical)
	}
	rule := doc[start:]
	if end := strings.Index(rule, "\n"); end >= 0 {
		rule = rule[:end]
	}
	assertOrder("stated", rule, footerClauseLabels, strings.Count(doc[:start], "\n")+1)
	for _, label := range footerClauseLabels {
		if !strings.Contains(rule, label) {
			t.Errorf("the canonical footer clause order must name %q", label)
		}
	}

	footers := 0
	for _, block := range loadSampleBlocks(t) {
		for _, sample := range block {
			if !strings.HasPrefix(sample.text, "_") || !strings.Contains(sample.text, "|Sippy>") {
				continue
			}
			footers++
			assertOrder("footer", sample.text, footerClauseOrder, sample.number)
			for _, link := range footerLinks {
				if !strings.Contains(sample.text, link) {
					t.Errorf("line %d: every footer carries the three links (missing %q)", sample.number, link)
				}
			}
			if strings.Contains(sample.text, "more categories") {
				t.Errorf("line %d: a footer routes to the complete breakdown; it never counts the categories it omits", sample.number)
			}
		}
	}
	if footers < 2 {
		t.Errorf("expected the format template footer and at least one worked example footer, found %d", footers)
	}
}

// TestDailyHealthReportBreakdownPointerExample asserts a worked example
// practises the pointer clause, and that the category count it teaches is the
// configured one. A stale count would demonstrate the one-row-per-category
// contract being broken.
func TestDailyHealthReportBreakdownPointerExample(t *testing.T) {
	configured := len(loadCIStatusConfig(t).Categories)
	examples := 0
	for _, block := range loadSampleBlocks(t) {
		for _, sample := range block {
			m := breakdownPointerRe.FindStringSubmatch(sample.text)
			if m == nil {
				continue
			}
			examples++
			n, err := strconv.Atoi(m[1])
			if err != nil {
				t.Errorf("line %d: unparsable category count %q", sample.number, m[1])
				continue
			}
			if n != configured {
				t.Errorf("line %d: example points at %d categories but the registry configures %d; update the example",
					sample.number, n, configured)
			}
		}
	}
	if examples == 0 {
		t.Error("expected a worked example whose footer carries the complete-breakdown pointer")
	}
}

// TestDailyHealthReportWithheldDetailKeepsAnalysis asserts that withholding
// detail from a summary line withholds only the text: the threading gate still
// follows the computed health, so a yellow or red category whose line now says
// nothing beyond "details withheld" still gets its mandatory step 5 reply.
func TestDailyHealthReportWithheldDetailKeepsAnalysis(t *testing.T) {
	doc := loadDailyHealthReportDoc(t)
	for _, needle := range []string{
		"did this run **compute** any :red_circle: or :large_yellow_circle: category",
		"The investigation belongs in the step 5 threaded replies.",
		"at least one **failure-analysis** reply section",
	} {
		if !strings.Contains(doc, needle) {
			t.Errorf("withholding summary detail must not suppress mandatory failure analysis (missing %q)", needle)
		}
	}
}
