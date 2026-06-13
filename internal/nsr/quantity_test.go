package nsr

import "testing"

func TestNwSizeBytes(t *testing.T) {
	cases := []struct {
		name string
		in   *nwSize
		want float64
		ok   bool
	}{
		{"nil absent", nil, 0, false},
		{"byte unit", &nwSize{Unit: "Byte", Value: 1680}, 1680, true},
		{"empty unit treated as bytes", &nwSize{Unit: "", Value: 42}, 42, true},
		{"kb scaled to bytes", &nwSize{Unit: "KB", Value: 1000}, 1024000, true},
		{"unknown unit absent", &nwSize{Unit: "GB", Value: 5}, 0, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := c.in.Bytes()
			if got != c.want || ok != c.ok {
				t.Fatalf("Bytes() = (%v,%v), want (%v,%v)", got, ok, c.want, c.ok)
			}
		})
	}
}

func TestNwBitRateBytesPerSecond(t *testing.T) {
	cases := []struct {
		name string
		in   *nwBitRate
		want float64
		ok   bool
	}{
		{"nil absent", nil, 0, false},
		{"bytes per sec", &nwBitRate{Unit: "Byte/s", Value: 500}, 500, true},
		{"kb per sec scaled", &nwBitRate{Unit: "KB/s", Value: 10}, 10240, true},
		{"unknown unit absent", &nwBitRate{Unit: "Mb/s", Value: 9}, 0, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := c.in.BytesPerSecond()
			if got != c.want || ok != c.ok {
				t.Fatalf("BytesPerSecond() = (%v,%v), want (%v,%v)", got, ok, c.want, c.ok)
			}
		})
	}
}

func TestParseHumanSize(t *testing.T) {
	cases := []struct {
		in   string
		want float64
		ok   bool
	}{
		{"112 GB", 112 * (1 << 30), true},
		{"202 MB", 202 * (1 << 20), true},
		{"0 KB", 0, true},
		{"2852 GB", 2852 * (1 << 30), true},
		{"365 TB", 365 * (1 << 40), true},
		{"1.5 TB", 1.5 * (1 << 40), true},
		{"", 0, false},
		{"112", 0, false},
		{"112 ZB", 0, false},
		{"abc GB", 0, false},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			got, ok := parseHumanSize(c.in)
			if got != c.want || ok != c.ok {
				t.Fatalf("parseHumanSize(%q) = (%v,%v), want (%v,%v)", c.in, got, ok, c.want, c.ok)
			}
		})
	}
}
