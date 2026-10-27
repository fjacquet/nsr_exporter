package nsr

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/sirupsen/logrus"

	"github.com/fjacquet/nsr_exporter/internal/models"
)

// Decisions (client_golang registry.go, checked for an unchecked collector):
//   - Suffix collisions: checkSuffixCollisions only fires for _count/_sum/_bucket
//     against a summary or histogram; we emit gauges and counters only, so it cannot
//     abort Gather and gets no reason.
//   - Self-metric name: the exporter-standards family defines no guard metric, so
//     nsr_guard_repaired_samples stays (nsr_ prefix, unit-free count of samples).

type guardReason string

const (
	reasonDuplicate    guardReason = "duplicate"
	reasonTypeMismatch guardReason = "type_mismatch"
	reasonHelpMismatch guardReason = "help_mismatch"
	reasonInvalidLabel guardReason = "invalid_label"
)

var guardReasons = []guardReason{reasonDuplicate, reasonTypeMismatch, reasonHelpMismatch, reasonInvalidLabel}

const guardMetric = "nsr_guard_repaired_samples"

var validLabelName = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

type nameState struct {
	typ  models.MetricType
	help string
}

// guardSamples repairs the classes of series that make the Prometheus registry
// abort Gather (and so /metrics return 500): duplicate series, type or help
// mismatch for one name, and invalid or repeated label names. The first-seen
// sample wins. It never mutates its input and appends one nsr_guard_repaired_samples
// gauge per (system, reason). Label-key-set differences are not aborting; untouched.
func guardSamples(samples []models.Sample, systems []string, log *logrus.Logger) []models.Sample {
	out := make([]models.Sample, 0, len(samples)+len(systems)*len(guardReasons))
	names := make(map[string]nameState)
	seen := make(map[string]struct{}, len(samples))
	counts := make(map[string]map[guardReason]int, len(systems))
	for _, sys := range systems {
		counts[sys] = map[guardReason]int{}
	}
	warned := map[string]struct{}{}
	repair := func(s models.Sample, r guardReason) {
		for _, l := range s.Labels {
			if l.Key == systemLabel {
				if c, ok := counts[l.Value]; ok {
					c[r]++
				}
			}
		}
		if _, ok := warned[s.Name+"|"+string(r)]; !ok {
			warned[s.Name+"|"+string(r)] = struct{}{}
			log.WithFields(logrus.Fields{"metric": s.Name, "reason": r}).Warn("series guard repaired a sample")
		}
	}

	for _, s := range samples {
		if !validLabels(s.Labels) {
			repair(s, reasonInvalidLabel)
			continue
		}
		st, known := names[s.Name]
		if !known {
			names[s.Name] = nameState{typ: s.Type, help: s.Help}
		} else if st.typ != s.Type {
			repair(s, reasonTypeMismatch)
			continue
		}
		key := seriesKey(s)
		if _, dup := seen[key]; dup {
			repair(s, reasonDuplicate)
			continue
		}
		seen[key] = struct{}{}
		if known && st.help != s.Help {
			repair(s, reasonHelpMismatch)
			s.Help = st.help
		}
		out = append(out, s)
	}

	for _, sys := range systems {
		for _, r := range guardReasons {
			out = append(out, models.Sample{
				Name: guardMetric, Type: models.Gauge,
				Help:   "Samples the series guard dropped or rewrote this cycle because they would abort /metrics.",
				Labels: []models.Label{lbl("reason", string(r)), lbl(systemLabel, sys)},
				Value:  float64(counts[sys][r]),
			})
		}
	}
	return out
}

func validLabels(labels []models.Label) bool {
	keys := make(map[string]struct{}, len(labels))
	for _, l := range labels {
		if !validLabelName.MatchString(l.Key) || strings.HasPrefix(l.Key, "__") || !utf8.ValidString(l.Value) {
			return false
		}
		if _, dup := keys[l.Key]; dup {
			return false
		}
		keys[l.Key] = struct{}{}
	}
	return true
}

// seriesKey identifies a series by name plus label pairs sorted by key.
func seriesKey(s models.Sample) string {
	ls := append([]models.Label(nil), s.Labels...)
	sort.Slice(ls, func(i, j int) bool { return ls[i].Key < ls[j].Key })
	var b strings.Builder
	b.WriteString(s.Name)
	for _, l := range ls {
		fmt.Fprintf(&b, "\x00%s\x01%s", l.Key, l.Value)
	}
	return b.String()
}
