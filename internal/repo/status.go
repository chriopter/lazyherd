package repo

import (
	"strconv"
	"strings"
)

// Change is one entry of git status: a file with its index and worktree state.
type Change struct {
	Path      string
	Staged    byte // X column of git status, '.' when clean
	Unstaged  byte // Y column
	Untracked bool
}

// Code is the two-letter status as git shows it, e.g. "M ", " M", "??".
func (c Change) Code() string {
	if c.Untracked {
		return "??"
	}
	return string([]byte{dot(c.Staged), dot(c.Unstaged)})
}

func dot(b byte) byte {
	if b == '.' {
		return ' '
	}
	return b
}

// Status is everything one git status call tells about a repository.
type Status struct {
	Head       string // commit id
	Committed  int64  // unix time of the HEAD commit, 0 when unborn
	Branch     string // branch name, or "@<short id>" when detached
	Ahead      int
	Behind     int
	NoUpstream bool
	Changes    []Change
}

// status reads the repository state with a single git call.
func status(dir string) (Status, error) {
	out, err := git(gitTimeout, dir, "status", "--porcelain=v2", "--branch", "--untracked-files=all", "-z")
	if err != nil {
		return Status{}, err
	}
	st := parseStatus(out)
	if st.Head != "" && st.Head != "(initial)" {
		if ts, err := git(gitTimeout, dir, "log", "-1", "--format=%ct", "HEAD"); err == nil {
			st.Committed, _ = strconv.ParseInt(strings.TrimSpace(ts), 10, 64)
		}
	}
	return st, nil
}

// Ago formats a unix timestamp the way lazygit does: "5m", "3h", "2d", "1w", "4M", "1y".
func Ago(now, timestamp int64) string {
	if timestamp == 0 {
		return ""
	}
	secs := now - timestamp
	if secs < 0 {
		secs = 0
	}
	units := []struct {
		label string
		secs  int64
	}{{"s", 1}, {"m", 60}, {"h", 3600}, {"d", 86400}, {"w", 604800}, {"M", 31536000 / 12}, {"y", 31536000}}
	for i := 1; i < len(units); i++ {
		if secs < units[i].secs {
			return strconv.FormatInt(secs/units[i-1].secs, 10) + units[i-1].label
		}
	}
	last := units[len(units)-1]
	return strconv.FormatInt(secs/last.secs, 10) + last.label
}

// parseStatus parses git status --porcelain=v2 --branch output. With -z the
// entries are NUL separated and paths come unquoted; a rename's old path
// follows as a separate NUL field. Newline separated input is accepted too.
func parseStatus(out string) Status {
	st := Status{NoUpstream: true}
	var entries []string
	if strings.Contains(out, "\x00") {
		entries = strings.Split(out, "\x00")
	} else {
		entries = strings.Split(out, "\n")
	}
	for i := 0; i < len(entries); i++ {
		line := entries[i]
		if line == "" {
			continue
		}
		if line[0] == '2' && strings.Contains(out, "\x00") {
			i++ // skip the rename's original path
		}
		switch line[0] {
		case '#':
			key, val, _ := strings.Cut(strings.TrimPrefix(line, "# "), " ")
			switch key {
			case "branch.oid":
				st.Head = val
			case "branch.head":
				st.Branch = val
			case "branch.ab":
				st.NoUpstream = false
				a, b, _ := strings.Cut(val, " ")
				st.Ahead, _ = strconv.Atoi(strings.TrimPrefix(a, "+"))
				st.Behind, _ = strconv.Atoi(strings.TrimPrefix(b, "-"))
			}
		case '1', '2', 'u':
			if c, ok := parseChange(line); ok {
				st.Changes = append(st.Changes, c)
			}
		case '?':
			st.Changes = append(st.Changes, Change{Path: line[2:], Untracked: true})
		}
	}
	if st.Branch == "(detached)" || st.Branch == "" {
		st.Branch = "@" + short(st.Head)
	}
	return st
}

// parseChange decodes the tracked-file lines of porcelain v2: type 1 has 8
// fields before the path, type 2 (rename) 9 and the new path before a tab,
// type u (unmerged) 10.
func parseChange(line string) (Change, bool) {
	fields := strings.SplitN(line, " ", 11)
	if len(fields) < 3 || len(fields[1]) < 2 {
		return Change{}, false
	}
	c := Change{Staged: fields[1][0], Unstaged: fields[1][1]}
	var n int
	switch line[0] {
	case '1':
		n = 8
	case '2':
		n = 9
	default:
		n = 10
	}
	if len(fields) <= n {
		return Change{}, false
	}
	c.Path = strings.Join(fields[n:], " ")
	if line[0] == '2' {
		c.Path, _, _ = strings.Cut(c.Path, "\t") // newline format: "new\told"
	}
	return c, true
}

func short(id string) string {
	if len(id) > 7 {
		return id[:7]
	}
	return id
}
