package main

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var sepRe = regexp.MustCompile("[\\\\/]+")

var winRootRe = regexp.MustCompile("^[A-Za-z]:")

var restText = "(?P<rest>(?:\\\\\\\\|[\\\\/]|[^\\s\"'`,;<>|?*\\r\\n])*)"

var restJSON = "(?P<rest>[^\"\\r\\n]*)"

// ----- path helpers -----
func segSplit(p, hint string) (string, []string) {
	p = strings.TrimSpace(p)
	p = strings.Trim(p, "\"")
	kind := ""
	if winRootRe.MatchString(p) || strings.HasPrefix(p, "\\\\") {
		kind = "win"
	} else if strings.HasPrefix(p, "/") {
		kind = "posix"
	} else if hint == "windows" {
		kind = "win"
	} else {
		kind = "posix"
	}
	parts := sepRe.Split(p, -1)
	segs := []string{}
	for _, s := range parts {
		if s != "" {
			segs = append(segs, s)
		}
	}
	return kind, segs
}

func renderPath(kind string, segs []string, isJSON bool) string {
	if kind == "win" {
		sep := "\\"
		if isJSON {
			sep = "\\\\"
		}
		return strings.Join(segs, sep)
	}
	return "/" + strings.Join(segs, "/")
}

// ----- token -----
type Token struct {
	name, src, srcOS, dst string
	reText, reJSON        *regexp.Regexp
}

func newToken(name, src, srcOS, dst string) *Token {
	t := &Token{name: name, src: src, srcOS: srcOS, dst: dst}
	if src == "" || dst == "" {
		return t
	}
	kind, segs := segSplit(src, srcOS)
	formset := map[string]bool{}
	if kind == "win" {
		formset[strings.Join(segs, "\\")] = true
		formset[strings.Join(segs, "\\\\")] = true
		formset[strings.Join(segs, "/")] = true
	} else {
		formset["/"+strings.Join(segs, "/")] = true
	}
	forms := []string{}
	for f := range formset {
		forms = append(forms, f)
	}
	sort.Slice(forms, func(i, j int) bool { return len(forms[i]) > len(forms[j]) })
	alts := []string{}
	for _, f := range forms {
		alts = append(alts, regexp.QuoteMeta(f))
	}
	alt := "(?i)(?:" + strings.Join(alts, "|") + ")"
	t.reText = regexp.MustCompile(alt + restText)
	t.reJSON = regexp.MustCompile(alt + restJSON)
	return t
}

func (t *Token) sub(text string, isJSON bool) string {
	if t.reText == nil {
		return text
	}
	re := t.reText
	if isJSON {
		re = t.reJSON
	}
	dk, dsegs := segSplit(t.dst, "")
	var b strings.Builder
	last := 0
	for _, m := range re.FindAllStringSubmatchIndex(text, -1) {
		b.WriteString(text[last:m[0]])
		rest := ""
		if len(m) >= 4 && m[2] >= 0 {
			rest = text[m[2]:m[3]]
		}
		restSegs := []string{}
		for _, s := range sepRe.Split(rest, -1) {
			if s != "" {
				restSegs = append(restSegs, s)
			}
		}
		all := append(append([]string{}, dsegs...), restSegs...)
		b.WriteString(renderPath(dk, all, isJSON))
		last = m[1]
	}
	b.WriteString(text[last:])
	return b.String()
}

func orderTokens(tokens []*Token) []*Token {
	out := []*Token{}
	for _, t := range tokens {
		if t.src != "" && t.dst != "" {
			out = append(out, t)
		}
	}
	sort.Slice(out, func(i, j int) bool { return len(out[i].src) > len(out[j].src) })
	return out
}

func rewriteFile(path string, tokens []*Token) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	orig := string(data)
	isJSON := strings.HasSuffix(strings.ToLower(path), ".json") || strings.HasSuffix(strings.ToLower(path), ".jsonl")
	text := orig
	for _, t := range tokens {
		text = t.sub(text, isJSON)
	}
	for _, t := range tokens {
		if t.src != "" && t.dst != "" {
			text = replaceFold(text, mungeCC(t.src), mungeCC(t.dst))
		}
	}
	if text != orig {
		os.WriteFile(path, []byte(text), 0644)
	}
}

func rewriteTree(root string, tokens []*Token) {
	filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && looksLikeText(p) {
			rewriteFile(p, tokens)
		}
		return nil
	})
}

// mungeCC reproduces Claude Code's project-directory naming: every character
// that is not [A-Za-z0-9] becomes '-'. Claude resolves a session transcript at
// .claude/projects/<mungeCC(cwd)>/<sessionId>.jsonl, so after a cross-machine
// import these directory names must be renamed to match the rewritten cwd —
// otherwise resuming fails with "No conversation found with session ID ...".
func mungeCC(p string) string {
	var b strings.Builder
	for _, r := range p {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	return b.String()
}

// replaceFold is a case-insensitive literal string replacement.
func replaceFold(s, from, to string) string {
	if from == "" || from == to {
		return s
	}
	lf := strings.ToLower(from)
	ls := strings.ToLower(s)
	var b strings.Builder
	i := 0
	for {
		j := strings.Index(ls[i:], lf)
		if j < 0 {
			b.WriteString(s[i:])
			return b.String()
		}
		b.WriteString(s[i : i+j])
		b.WriteString(to)
		i += j + len(from)
	}
}

// renameMungedProjectDirs renames the children of every "projects" directory
// under root whose names encode a source-machine absolute path (munged to
// dashes) so they match this machine's paths. Covers both the embedded
// per-session .claude homes inside Cowork sessions and the Claude Code home.
func renameMungedProjectDirs(root string, tokens []*Token) {
	type mp struct{ from, to string }
	maps := []mp{}
	for _, t := range tokens {
		if t.src == "" || t.dst == "" {
			continue
		}
		f, d := mungeCC(t.src), mungeCC(t.dst)
		if !strings.EqualFold(f, d) {
			maps = append(maps, mp{f, d})
		}
	}
	if len(maps) == 0 {
		return
	}
	sort.Slice(maps, func(i, j int) bool { return len(maps[i].from) > len(maps[j].from) })
	filepath.WalkDir(root, func(p string, dd os.DirEntry, err error) error {
		if err != nil || !dd.IsDir() || dd.Name() != "projects" {
			return nil
		}
		ents, _ := os.ReadDir(p)
		for _, e := range ents {
			if !e.IsDir() {
				continue
			}
			n := e.Name()
			for _, m := range maps {
				if len(n) >= len(m.from) && strings.EqualFold(n[:len(m.from)], m.from) {
					nn := m.to + n[len(m.from):]
					if nn != n {
						oldP, newP := filepath.Join(p, n), filepath.Join(p, nn)
						if !exists(newP) {
							os.Rename(oldP, newP)
						} else {
							// target already exists (e.g. a retry created it):
							// move over anything it doesn't have yet.
							if kids, _ := os.ReadDir(oldP); kids != nil {
								for _, k := range kids {
									if !exists(filepath.Join(newP, k.Name())) {
										os.Rename(filepath.Join(oldP, k.Name()), filepath.Join(newP, k.Name()))
									}
								}
							}
							os.Remove(oldP)
						}
					}
					break
				}
			}
		}
		return nil
	})
}

func norm(p string) string {
	if p == "" {
		return p
	}
	p = strings.ReplaceAll(p, "\\", "/")
	return strings.ToLower(strings.TrimRight(p, "/"))
}

func isSub(child, parent string) bool {
	if child == "" || parent == "" {
		return false
	}
	c, pa := norm(child), norm(parent)
	return c == pa || strings.HasPrefix(c, pa+"/")
}

func rewritePathString(p string, tokens []*Token) string {
	for _, t := range tokens {
		p = t.sub(p, false)
	}
	return p
}

func parentOf(p string) string {
	kind, segs := segSplit(p, "")
	if len(segs) <= 1 {
		return p
	}
	return renderPath(kind, segs[:len(segs)-1], false)
}

func joinDest(base, name string) string {
	kind, segs := segSplit(base, "")
	segs = append(segs, name)
	return renderPath(kind, segs, false)
}

var winPathRe2 = regexp.MustCompile("[A-Za-z]:(?:\\\\\\\\|\\\\|/)[^\"'<>|?*\\r\\n;,]+")

var posixPathRe2 = regexp.MustCompile("/(?:Users|home)/[^\"'<>|?*\\r\\n;,:]+")

func cleanPath2(p string) string {
	p = strings.Trim(strings.TrimSpace(p), "\"'")
	p = strings.ReplaceAll(p, "\\\\", "\\")
	p = strings.TrimRight(p, ").,;]")
	return p
}
