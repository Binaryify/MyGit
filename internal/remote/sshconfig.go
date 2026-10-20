package remote

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// ConfigHosts are the hosts ~/.ssh/config names, and the files it
// includes, as ssh reads them: each Host's names that aren't patterns
// (with * or ?) or negated (!), in order, once each.
func ConfigHosts(home string) []string {
	var hosts []string
	seen := map[string]bool{}
	var read func(path string, depth int)
	read = func(path string, depth int) {
		f, err := os.Open(path)
		if err != nil || depth > 8 { // an Include of itself ends
			return
		}
		defer f.Close()
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			key, args := configLine(sc.Text())
			switch strings.ToLower(key) {
			case "host":
				for _, h := range args {
					if !strings.ContainsAny(h, "*?!") && !seen[h] {
						seen[h] = true
						hosts = append(hosts, h)
					}
				}
			case "include":
				for _, a := range args {
					for _, p := range includePaths(home, a) {
						read(p, depth+1)
					}
				}
			}
		}
	}
	read(filepath.Join(home, ".ssh", "config"), 0)
	return hosts
}

// configLine reads a line of ssh's config: a keyword and its arguments,
// "Keyword args" or "Keyword=args", with quoted arguments.
func configLine(line string) (string, []string) {
	line = strings.TrimSpace(line)
	if line == "" || line[0] == '#' {
		return "", nil
	}
	i := strings.IndexAny(line, " \t=")
	if i < 0 {
		return line, nil
	}
	key, rest := line[:i], strings.TrimLeft(line[i:], " \t")
	rest = strings.TrimLeft(strings.TrimPrefix(rest, "="), " \t")
	var args []string
	for rest != "" {
		var arg string
		if rest[0] == '"' {
			end := strings.IndexByte(rest[1:], '"')
			if end < 0 {
				arg, rest = rest[1:], ""
			} else {
				arg, rest = rest[1:end+1], rest[end+2:]
			}
		} else if j := strings.IndexAny(rest, " \t"); j >= 0 {
			arg, rest = rest[:j], rest[j:]
		} else {
			arg, rest = rest, ""
		}
		if arg != "" {
			args = append(args, arg)
		}
		rest = strings.TrimLeft(rest, " \t")
	}
	return key, args
}

// includePaths are the files an Include names: ~ is the home folder, a
// relative path is in ~/.ssh, and patterns match files.
func includePaths(home, p string) []string {
	if rest, ok := strings.CutPrefix(p, "~/"); ok {
		p = filepath.Join(home, rest)
	} else if !filepath.IsAbs(p) {
		p = filepath.Join(home, ".ssh", p)
	}
	matches, _ := filepath.Glob(p)
	return matches
}
