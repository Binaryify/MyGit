package main

import (
	"context"
	"os"
	"runtime"
	"testing"

	"github.com/egoist/mygo/ui/editor"

	"mygit/internal/git"
	"mygit/internal/graph"
)

// The benchmarks read a real repository, MYGIT_BENCH_REPO, and the commit
// MYGIT_BENCH_COMMIT for the diff:
//
//	MYGIT_BENCH_REPO=~/src/linux MYGIT_BENCH_COMMIT=abc123 go test -bench . -run ^$
func benchRepo(b *testing.B) *git.Repo {
	dir := os.Getenv("MYGIT_BENCH_REPO")
	if dir == "" {
		b.Skip("MYGIT_BENCH_REPO not set")
	}
	r, err := git.Open(dir)
	if err != nil {
		b.Fatal(err)
	}
	return r
}

// BenchmarkLoadHistory reads the whole history and lays out its graph, as
// opening the repository does, and reports the heap it keeps.
func BenchmarkLoadHistory(b *testing.B) {
	r := benchRepo(b)
	for range b.N {
		var commits []git.Commit
		var rows []graph.Row
		var l graph.Layout
		err := r.Log(context.Background(), git.Scope{}, logBatch, func(batch []git.Commit) {
			for _, c := range batch {
				rows = append(rows, l.Add(c.Hash, c.Parents))
			}
			commits = append(commits, batch...)
		})
		if err != nil {
			b.Fatal(err)
		}
		b.ReportMetric(float64(len(commits)), "commits")
		b.ReportMetric(heapMB(), "heapMB")
		runtime.KeepAlive(commits) // the app keeps both
		runtime.KeepAlive(rows)
	}
}

// BenchmarkShowCommit reads and parses a commit's patch and builds the
// diff of its largest file, as choosing it does.
func BenchmarkShowCommit(b *testing.B) {
	r := benchRepo(b)
	hash := os.Getenv("MYGIT_BENCH_COMMIT")
	if hash == "" {
		b.Skip("MYGIT_BENCH_COMMIT not set")
	}
	for range b.N {
		out, err := r.Show(context.Background(), hash, git.Whitespace{})
		if err != nil {
			b.Fatal(err)
		}
		files, err := editor.ParsePatch(out)
		if err != nil {
			b.Fatal(err)
		}
		biggest := 0
		for i, f := range files {
			if len(f.Hunks) > 0 && (len(files[biggest].Hunks) == 0 || lines(f) > lines(files[biggest])) {
				biggest = i
			}
		}
		d := patchDiff(files[biggest])
		b.ReportMetric(float64(len(out))/(1<<20), "patchMB")
		b.ReportMetric(float64(len(files)), "files")
		b.ReportMetric(float64(lines(files[biggest])), "biggestLines")
		b.ReportMetric(heapMB(), "heapMB")
		runtime.KeepAlive(d) // the app keeps the diff and the files
		runtime.KeepAlive(files)
	}
}

func lines(f editor.FilePatch) int {
	n := 0
	for _, h := range f.Hunks {
		n += len(h.Lines)
	}
	return n
}

func heapMB() float64 {
	runtime.GC()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return float64(m.HeapAlloc) / (1 << 20)
}
