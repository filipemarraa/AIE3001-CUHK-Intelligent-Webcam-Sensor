package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReplayTableNamesTheFileThatFails(t *testing.T) {
	_, err := replayTable([]string{filepath.Join(t.TempDir(), "missing.jsonl")})
	if err == nil || !strings.Contains(err.Error(), "missing.jsonl") {
		t.Fatalf("error %v, want one naming the file", err)
	}
	bad := filepath.Join(t.TempDir(), "bad.jsonl")
	if err := os.WriteFile(bad, []byte(`{"kind":"sample"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := replayTable([]string{bad}); err == nil || !strings.Contains(err.Error(), "bad.jsonl: line 1") {
		t.Fatalf("error %v, want one naming the file and the line", err)
	}
}

func TestReplayTableHasARowPerSkinRegion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "p1.jsonl")
	text := `{"kind":"header","version":1,"subject":"P1","condition":"good light"}
{"kind":"sample","v":[0,180,140,120,1,0.01,150,0.02,0.6,0.3],"skins":[180,140,120,181,141,121]}
{"kind":"reference","tMs":0,"hrBpm":70,"source":"watch"}
{"kind":"diagnostics","tMs":1000,"frameCostMs":20,"droppedFrames":0}
`
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	table, err := replayTable([]string{path})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"| p1.jsonl | P1 | good light | 100% | boxes | 1 | 0% |", "| p1.jsonl | P1 | good light | 100% | anatomical | 1 | 0% |", "SNR < 0 dB"} {
		if !strings.Contains(table, want) {
			t.Errorf("table lacks %q:\n%s", want, table)
		}
	}
}
