package main

import (
	"encoding/json"
	"testing"
	"time"
)

func TestJobsFixtureIsRecent(t *testing.T) {
	var body struct {
		Jobs []struct {
			StartTime string `json:"startTime"`
			EndTime   string `json:"endTime"`
		} `json:"jobs"`
	}
	if err := json.Unmarshal([]byte(fixtures["/jobs"]), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Jobs) == 0 {
		t.Fatal("no jobs")
	}
	for _, j := range body.Jobs {
		st, err := time.Parse(time.RFC3339, j.StartTime)
		if err != nil {
			t.Fatal(err)
		}
		if age := time.Since(st); age < 0 || age > 24*time.Hour {
			t.Fatalf("startTime %s age %v, want within last 24h", j.StartTime, age)
		}
	}
}
