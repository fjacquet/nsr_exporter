package nsr

import (
	"strconv"
	"strings"
)

// nwSize is NetWorker's Size model: a {"unit","value"} object, NOT a scalar.
// Decoding such a field into a plain float silently yields nil/zero with no error,
// so every byte-valued field MUST decode into this and convert via Bytes(). The
// unit enum the API emits is "Byte" or "KB" (swagger 19.13 Size model).
type nwSize struct {
	Unit  string  `json:"unit"`
	Value float64 `json:"value"`
}

// Bytes returns the value normalized to bytes and whether a usable value was
// present. A nil pointer or unknown unit yields ok=false so the caller emits no
// sample rather than a wrong or fake-zero one (ADR-0008).
func (s *nwSize) Bytes() (float64, bool) {
	if s == nil {
		return 0, false
	}
	switch s.Unit {
	case "KB":
		return s.Value * 1024, true
	case "Byte", "":
		return s.Value, true
	default:
		return 0, false
	}
}

// nwBitRate is NetWorker's BitRate model: a {"unit","value"} object with unit
// "Byte/s" or "KB/s" (swagger 19.13). Per-second values are gauges.
type nwBitRate struct {
	Unit  string  `json:"unit"`
	Value float64 `json:"value"`
}

// BytesPerSecond normalizes the rate to bytes/second, ok=false when absent or the
// unit is unrecognized.
func (r *nwBitRate) BytesPerSecond() (float64, bool) {
	if r == nil {
		return 0, false
	}
	switch r.Unit {
	case "KB/s":
		return r.Value * 1024, true
	case "Byte/s", "":
		return r.Value, true
	default:
		return 0, false
	}
}

// humanSizeUnits maps the suffixes NetWorker uses in human-readable capacity
// strings (e.g. DataDomain "112 GB") to their byte multiplier. NetWorker reports
// these in IEC magnitudes (1 KB = 1024 B) despite the SI-style suffix.
var humanSizeUnits = map[string]float64{
	"B":  1,
	"KB": 1 << 10,
	"MB": 1 << 20,
	"GB": 1 << 30,
	"TB": 1 << 40,
	"PB": 1 << 50,
	"EB": 1 << 60,
}

// parseHumanSize converts a human-readable capacity string such as "112 GB",
// "202 MB", or "0 KB" into bytes. It returns ok=false for empty, malformed, or
// unknown-unit input so the caller emits no sample rather than a wrong value.
func parseHumanSize(s string) (float64, bool) {
	fields := strings.Fields(s)
	if len(fields) != 2 {
		return 0, false
	}
	value, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return 0, false
	}
	mult, ok := humanSizeUnits[strings.ToUpper(fields[1])]
	if !ok {
		return 0, false
	}
	return value * mult, true
}
