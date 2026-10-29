//go:build ignore

// Gen writes pinyin.bin, the Mandarin readings that slugify gives Chinese
// characters, from the kMandarin field of Unicode's Unihan database:
// `go generate ./cmd/mygo`. Its first line is the readings without tones,
// separated by spaces; then, for each code point of CJK Unified Ideographs
// (U+4E00 to U+9FFF), its reading's position on that line from 1, as a
// big-endian uint16, or 0 without a reading.
package main

import (
	"archive/zip"
	"bufio"
	"bytes"
	"encoding/binary"
	"io"
	"log"
	"net/http"
	"os"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

const (
	unihan     = "https://www.unicode.org/Public/16.0.0/ucd/Unihan.zip"
	first, end = 0x4E00, 0xA000
)

func main() {
	log.SetFlags(0)
	resp, err := http.Get(unihan)
	if err != nil {
		log.Fatal(err)
	}
	data, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil || resp.StatusCode != http.StatusOK {
		log.Fatalf("downloading %s: %v %s", unihan, err, resp.Status)
	}
	z, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		log.Fatal(err)
	}
	f, err := z.Open("Unihan_Readings.txt")
	if err != nil {
		log.Fatal(err)
	}
	readings := make([]string, end-first)
	lines := bufio.NewScanner(f)
	for lines.Scan() {
		field := strings.Split(lines.Text(), "\t")
		if len(field) != 3 || field[1] != "kMandarin" {
			continue
		}
		r, err := strconv.ParseUint(strings.TrimPrefix(field[0], "U+"), 16, 32)
		if err != nil {
			log.Fatal(err)
		}
		if r < first || r >= end {
			continue
		}
		// The first reading is the most common one, without its tone: the
		// marks above its letters, as of ǚ.
		reading := strings.Map(func(r rune) rune {
			if unicode.Is(unicode.Mn, r) {
				return -1
			}
			return r
		}, norm.NFD.String(strings.Fields(field[2])[0]))
		if strings.Trim(reading, "abcdefghijklmnopqrstuvwxyz") != "" {
			log.Fatalf("%s reads %q", field[0], reading)
		}
		readings[r-first] = reading
	}
	if err := lines.Err(); err != nil {
		log.Fatal(err)
	}

	var syllables []string
	for _, r := range readings {
		if r != "" && !slices.Contains(syllables, r) {
			syllables = append(syllables, r)
		}
	}
	slices.Sort(syllables)
	out := []byte(strings.Join(syllables, " ") + "\n")
	for _, r := range readings {
		i := 0
		if r != "" {
			i, _ = slices.BinarySearch(syllables, r)
			i++
		}
		out = binary.BigEndian.AppendUint16(out, uint16(i))
	}
	if err := os.WriteFile("pinyin.bin", out, 0o644); err != nil {
		log.Fatal(err)
	}
}
