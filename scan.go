package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
)

func defaultRoots(osn, home string) (string, string, string) {
	if osn == "windows" {
		ad := os.Getenv("APPDATA")
		if ad == "" {
			ad = filepath.Join(home, "AppData", "Roaming")
		}
		return filepath.Join(ad, "Claude"), filepath.Join(home, ".claude"), filepath.Join(home, ".claude.json")
	}
	return filepath.Join(home, "Library", "Application Support", "Claude"),
		filepath.Join(home, ".claude"), filepath.Join(home, ".claude.json")
}

func driveRoots() []string {
	if runtime.GOOS == "windows" {
		out := []string{}
		for c := 'A'; c <= 'Z'; c++ {
			d := string(c) + ":" + string(os.PathSeparator)
			if _, err := os.Stat(d); err == nil {
				out = append(out, d)
			}
		}
		if len(out) == 0 {
			out = append(out, "C:"+string(os.PathSeparator))
		}
		return out
	}
	out := []string{"/"}
	if h, err := os.UserHomeDir(); err == nil {
		out = append(out, h)
	}
	if ents, err := os.ReadDir("/Volumes"); err == nil {
		for _, e := range ents {
			if e.IsDir() {
				out = append(out, filepath.Join("/Volumes", e.Name()))
			}
		}
	}
	return out
}

var reParenNum = regexp.MustCompile(`\s*\(\d+\)$`)

func bestFolderLabel(folders map[string]bool) string {
	pick := func(req bool) string {
		best := ""
		bd := -1
		for f := range folders {
			if isAppish(f) {
				continue
			}
			if req && !isDir(f) {
				continue
			}
			n := strings.Count(filepath.ToSlash(strings.TrimRight(f, "/\\")), "/")
			if n > bd {
				bd = n
				best = f
			}
		}
		return best
	}
	best := pick(true)
	if best == "" {
		best = pick(false)
	}
	if best == "" {
		return ""
	}
	b := filepath.Base(strings.TrimRight(best, "/\\"))
	return strings.TrimSpace(reParenNum.ReplaceAllString(b, ""))
}

func isAppish(p string) bool {
	pl := strings.ToLower(strings.ReplaceAll(p, "\\", "/"))
	for _, m := range []string{"/appdata/", "/application support/", "/library/caches/", "/caches/", "claude_pzs8sxrjxfjjc", "/.claude/", "/.cache/", "/localcache/"} {
		if strings.Contains(pl, m) {
			return true
		}
	}
	return strings.HasSuffix(pl, "/.claude")
}

func scanFileForPaths(fp string, add func(string)) {
	if !looksLikeText(fp) {
		return
	}
	b, err := os.ReadFile(fp)
	if err != nil {
		return
	}
	str := string(b)
	for _, m := range winPathRe2.FindAllString(str, -1) {
		add(cleanPath2(m))
	}
	for _, m := range posixPathRe2.FindAllString(str, -1) {
		add(cleanPath2(m))
	}
}

func detectConnected(coworkRoot, ccjson, home string) []string {
	set := map[string]bool{}
	add := func(p string) {
		if p != "" && isDir(p) {
			set[p] = true
		}
	}
	if m := readJSON(ccjson); m != nil {
		if pr, ok := m["projects"].(map[string]interface{}); ok {
			for k := range pr {
				add(k)
			}
		}
	}
	if coworkRoot != "" && isDir(coworkRoot) {
		scanFileForPaths(filepath.Join(coworkRoot, "claude_desktop_config.json"), add)
		cnt := 0
		filepath.WalkDir(filepath.Join(coworkRoot, "local-agent-mode-sessions"), func(p string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() || cnt > 4000 {
				return nil
			}
			if looksLikeText(p) {
				cnt++
				scanFileForPaths(p, add)
			}
			return nil
		})
	}
	out := []string{}
	cc := filepath.Join(home, ".claude")
	for p := range set {
		if !(isAppish(p) || isSub(p, coworkRoot) || isSub(p, cc)) {
			out = append(out, p)
		}
	}
	sort.Strings(out)
	return out
}

type connFolder struct {
	Path   string `json:"path"`
	Exists bool   `json:"exists"`
	Bytes  int64  `json:"bytes"`
	Files  int    `json:"files"`
}

type connProject struct {
	Scope   string       `json:"scope"`
	Name    string       `json:"name"`
	Folders []connFolder `json:"folders"`
}

func mkConnFolder(p string, withSize bool) connFolder {
	cf := connFolder{Path: p, Exists: isDir(p)}
	if withSize && cf.Exists {
		f, b, _ := folderStats(p)
		cf.Files = f
		cf.Bytes = b
	}
	return cf
}

func projectNames(coworkRoot string) map[string]string {
	res := map[string]string{}
	if coworkRoot == "" {
		return res
	}
	skip := map[string]bool{"vm_bundles": true, "node_modules": true, "Cache": true, "GPUCache": true, "Code Cache": true, "Local Storage": true, "IndexedDB": true, "Network": true, "Service Worker": true, "blob_storage": true}
	getName := func(m map[string]interface{}) string {
		for _, k := range []string{"name", "title", "displayName", "projectName"} {
			if v, ok := m[k].(string); ok && strings.TrimSpace(v) != "" {
				return strings.TrimSpace(v)
			}
		}
		return ""
	}
	put := func(m map[string]interface{}, nm, dirID string) {
		if nm == "" {
			return
		}
		if dirID != "" {
			res[dirID] = nm
		}
		for _, k := range []string{"id", "spaceId", "projectId", "uuid", "projectUuid", "space_id"} {
			if v, ok := m[k].(string); ok && v != "" {
				res[v] = nm
			}
		}
	}
	filepath.WalkDir(coworkRoot, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if skip[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		lp := strings.ToLower(filepath.ToSlash(p))
		if !strings.HasSuffix(lp, "metadata.json") || !strings.Contains(lp, "project-cache/") {
			return nil
		}
		m := readJSON(p)
		if m == nil {
			return nil
		}
		put(m, getName(m), filepath.Base(filepath.Dir(p)))
		return nil
	})
	return res
}

func enumerateConnections(coworkRoot, ccRoot, ccjson string, withSize bool) []connProject {
	out := []connProject{}
	names := projectNames(coworkRoot)
	type sp struct {
		titles  []string
		folders map[string]bool
	}
	spaces := map[string]*sp{}
	order := []string{}
	if coworkRoot != "" && isDir(coworkRoot) {
		filepath.WalkDir(filepath.Join(coworkRoot, "local-agent-mode-sessions"), func(p string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			n := d.Name()
			if !(strings.HasPrefix(n, "local_") && strings.HasSuffix(n, ".json")) {
				return nil
			}
			m := readJSON(p)
			if m == nil {
				return nil
			}
			sid, _ := m["spaceId"].(string)
			if sid == "" {
				sid = "(unsorted)"
			}
			g := spaces[sid]
			if g == nil {
				g = &sp{folders: map[string]bool{}}
				spaces[sid] = g
				order = append(order, sid)
			}
			if t, ok := m["title"].(string); ok && t != "" {
				g.titles = append(g.titles, t)
			}
			if usf, ok := m["userSelectedFolders"].([]interface{}); ok {
				for _, x := range usf {
					if ps, ok := x.(string); ok && ps != "" && !isAppish(ps) {
						g.folders[ps] = true
					}
				}
			}
			return nil
		})
	}
	for _, sid := range order {
		g := spaces[sid]
		name := "Cowork project"
		if sid == "(unsorted)" {
			name = "Loose chats (no project)"
		}
		if len(g.titles) > 0 {
			name = g.titles[len(g.titles)-1]
		}
		if fl := bestFolderLabel(g.folders); fl != "" {
			name = fl
		}
		if n, ok := names[sid]; ok && n != "" {
			name = n
		}
		fs := []connFolder{}
		for f := range g.folders {
			fs = append(fs, mkConnFolder(f, withSize))
		}
		sort.Slice(fs, func(i, j int) bool { return fs[i].Path < fs[j].Path })
		out = append(out, connProject{Scope: "Cowork", Name: name, Folders: fs})
	}
	codeSet := map[string]bool{}
	if coworkRoot != "" {
		filepath.WalkDir(filepath.Join(coworkRoot, "claude-code-sessions"), func(p string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(d.Name(), ".json") {
				return nil
			}
			if m := readJSON(p); m != nil {
				if cwd, ok := m["cwd"].(string); ok && cwd != "" && !isAppish(cwd) {
					codeSet[cwd] = true
				}
			}
			return nil
		})
	}
	if m := readJSON(ccjson); m != nil {
		if pr, ok := m["projects"].(map[string]interface{}); ok {
			for k := range pr {
				if !isAppish(k) {
					codeSet[k] = true
				}
			}
		}
	}
	cl := []string{}
	for k := range codeSet {
		cl = append(cl, k)
	}
	sort.Strings(cl)
	for _, f := range cl {
		out = append(out, connProject{Scope: "Code", Name: filepath.Base(strings.TrimRight(f, "/\\")), Folders: []connFolder{mkConnFolder(f, withSize)}})
	}
	return out
}

func allConnectedFolders(coworkRoot, ccRoot, ccjson string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, g := range enumerateConnections(coworkRoot, ccRoot, ccjson, false) {
		for _, f := range g.Folders {
			if f.Exists && !seen[norm(f.Path)] {
				seen[norm(f.Path)] = true
				out = append(out, f.Path)
			}
		}
	}
	return out
}

func enumerateConnectors(coworkRoot, ccRoot, ccjson string) []string {
	set := map[string]bool{}
	addMcp := func(m map[string]interface{}) {
		if m == nil {
			return
		}
		if mc, ok := m["mcpServers"].(map[string]interface{}); ok {
			for k := range mc {
				set[k] = true
			}
		}
	}
	if coworkRoot != "" {
		addMcp(readJSON(filepath.Join(coworkRoot, "claude_desktop_config.json")))
	}
	addMcp(readJSON(ccjson))
	if ccRoot != "" {
		addMcp(readJSON(filepath.Join(ccRoot, "settings.json")))
	}
	out := []string{}
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func htmlEsc(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", "\"", "&quot;").Replace(s)
}

func mb(b int64) string { return fmt.Sprintf("%.1f MB", float64(b)/1e6) }

func connectionsHTML(groups []connProject, skills []map[string]string, connectors []string, host string) string {
	var b strings.Builder
	b.WriteString("<!doctype html><html><head><meta charset=\"utf-8\"><title>Claude Migrator - Connections</title><style>")
	b.WriteString("body{font-family:-apple-system,Segoe UI,Arial,sans-serif;background:#1d1f23;color:#e6e6e6;margin:0;padding:24px}")
	b.WriteString("h1{font-size:20px;margin:0 0 2px}.sz{color:#9aa0a6;font-size:12px}")
	b.WriteString(".tabs label{display:inline-block;padding:8px 16px;background:#2a2d31;border-radius:8px;margin:12px 6px 12px 0;cursor:pointer;font-weight:600}")
	b.WriteString("input[name=t]{display:none}.tab{display:none}")
	b.WriteString("#p:checked~#tp,#s:checked~#ts,#c:checked~#tc{display:block}")
	b.WriteString("#p:checked~.tabs label[for=p],#s:checked~.tabs label[for=s],#c:checked~.tabs label[for=c]{background:#d97757;color:#1d1f23}")
	b.WriteString(".scope{font-size:14px;color:#d97757;margin:16px 0 6px;font-weight:600}")
	b.WriteString(".proj{border:1px solid #2f333a;border-radius:8px;padding:10px 12px;margin:8px 0;background:#23262b}")
	b.WriteString(".nm{font-weight:600}.f{font-size:13px;color:#cfd3d7;margin:4px 0 0 14px}.loc{color:#9aa0a6}")
	b.WriteString("</style></head><body>")
	b.WriteString("<h1>Claude Migrator - connection index</h1>")
	b.WriteString("<div class=sz>Exported from " + htmlEsc(host) + ". Every project and the folders it is connected to, with on-disk locations (handy for manual transfer too).</div>")
	b.WriteString("<input type=radio name=t id=p checked><input type=radio name=t id=s><input type=radio name=t id=c>")
	b.WriteString("<div class=tabs><label for=p>Projects</label><label for=s>Skills</label><label for=c>Connectors</label></div>")
	b.WriteString("<div class=tab id=tp>")
	for _, scope := range []string{"Cowork", "Code"} {
		has := false
		for _, g := range groups {
			if g.Scope == scope {
				has = true
			}
		}
		if !has {
			continue
		}
		b.WriteString("<div class=scope>" + scope + " projects</div>")
		for _, g := range groups {
			if g.Scope != scope {
				continue
			}
			b.WriteString("<div class=proj><div class=nm>" + htmlEsc(g.Name) + "</div>")
			for _, f := range g.Folders {
				ex := ""
				if !f.Exists {
					ex = " (missing)"
				}
				b.WriteString("<div class=f>" + htmlEsc(f.Path) + " <span class=loc>- " + mb(f.Bytes) + ", " + fmt.Sprint(f.Files) + " files" + ex + "</span></div>")
			}
			b.WriteString("</div>")
		}
	}
	b.WriteString("</div><div class=tab id=ts>")
	if len(skills) == 0 {
		b.WriteString("<div class=f>None detected.</div>")
	}
	for _, sk := range skills {
		b.WriteString("<div class=proj><span class=nm>" + htmlEsc(sk["name"]) + "</span> <span class=loc>- " + htmlEsc(sk["type"]) + "</span><div class=f>" + htmlEsc(sk["path"]) + "</div></div>")
	}
	b.WriteString("</div><div class=tab id=tc>")
	if len(connectors) == 0 {
		b.WriteString("<div class=f>None detected.</div>")
	}
	for _, c := range connectors {
		b.WriteString("<div class=proj><span class=nm>" + htmlEsc(c) + "</span></div>")
	}
	b.WriteString("</div></body></html>")
	return b.String()
}
