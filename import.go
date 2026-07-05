package main

import (
	"archive/zip"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

var coworkMergeSkip = map[string]bool{"ChromeNativeHost": true, "buddy-tokens.json": true, "config.json": true, "Local Storage": true, "IndexedDB": true, "Session Storage": true, "Network": true, "Cookies": true, "Cookies-journal": true, "Cache": true, "Code Cache": true, "GPUCache": true, "DawnCache": true, "DawnGraphiteCache": true, "DawnWebGPUCache": true, "ShaderCache": true, "GrShaderCache": true, "blob_storage": true, "Service Worker": true, "Crashpad": true, "vm_bundles": true, ".credentials.json": true, ".audit-key": true, "Trust Tokens": true, "Trust Tokens-journal": true, "Network Persistent State": true}

func mergeClaudeJSON(staged, live string, tokens []*Token, cb func(string)) {
	s := readRaw(staged)
	for _, t := range tokens {
		s = t.sub(s, true)
	}
	var sm map[string]interface{}
	json.Unmarshal([]byte(s), &sm)
	base := readJSON(live)
	if base == nil {
		base = map[string]interface{}{}
	}
	for _, k := range []string{"projects", "mcpServers"} {
		if sv, ok := sm[k].(map[string]interface{}); ok {
			bv, _ := base[k].(map[string]interface{})
			if bv == nil {
				bv = map[string]interface{}{}
			}
			for kk, vv := range sv {
				bv[kk] = vv
			}
			base[k] = bv
		}
	}
	writeJSON(live, base)
	logln(cb, "    merged .claude.json (kept local account)")
}

// announceConnectors tells the user which connectors were deliberately NOT
// carried over (pairings are machine-bound) and pops the reconnect list in the UI.
func announceConnectors(core string, cb func(string)) {
	cj := readJSON(filepath.Join(core, "connectors.json"))
	if cj == nil {
		return
	}
	arr, _ := cj["connectors"].([]interface{})
	names := []string{}
	for _, x := range arr {
		if s, _ := x.(string); s != "" {
			names = append(names, s)
		}
	}
	if len(names) == 0 {
		return
	}
	logln(cb, "  Connectors are machine-bound and were NOT carried over (prevents this machine driving the old computer's browser).")
	logln(cb, "  Reconnect on this machine: %s", strings.Join(names, ", "))
	if cb != nil {
		b, _ := json.Marshal(names)
		cb("@@POP@@" + string(b))
	}
}

// probeLayout sanity-checks that Claude's on-disk layout still looks like what
// this version understands, so a Claude app update that changes the storage
// format produces a visible warning instead of a silently useless backup.
func probeLayout(cowork string) []string {
	warns := []string{}
	if !isDir(cowork) {
		return append(warns, "Claude Desktop data folder not found -- is Claude installed on this machine?")
	}
	lam := filepath.Join(cowork, "local-agent-mode-sessions")
	if !isDir(lam) {
		return append(warns, "Unexpected layout: no local-agent-mode-sessions inside the Claude folder. Claude's storage format may have changed -- check for a newer Claude Migrator.")
	}
	seen, parsed := 0, 0
	filepath.WalkDir(lam, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasPrefix(d.Name(), "local_") || !strings.HasSuffix(d.Name(), ".json") {
			return nil
		}
		seen++
		if m := readJSON(p); m != nil {
			if _, ok := m["sessionId"]; ok {
				parsed++
			} else if _, ok := m["title"]; ok {
				parsed++
			} else if _, ok := m["userSelectedFolders"]; ok {
				parsed++
			}
		}
		return nil
	})
	if seen == 0 {
		warns = append(warns, "No conversations found in Claude's data folder. If you know you have chats, Claude's storage format may have changed -- check for a newer Claude Migrator.")
	} else if parsed == 0 {
		warns = append(warns, "Conversations found but none match a known format. Claude's storage may have changed -- check for a newer Claude Migrator before trusting this backup.")
	}
	return warns
}

// verifyImport re-resolves every restored conversation exactly the way Claude
// does (projects/<munge(cwd)>/<cliSessionId>.jsonl) and reports whether it will
// actually resume, plus whether the connected data folders exist.
func verifyImport(coworkPhys string, cb func(string)) {
	lam := filepath.Join(coworkPhys, "local-agent-mode-sessions")
	tot, ok := 0, 0
	folders := map[string]bool{}
	filepath.WalkDir(lam, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasPrefix(d.Name(), "local_") || !strings.HasSuffix(d.Name(), ".json") {
			return nil
		}
		m := readJSON(p)
		if m == nil {
			return nil
		}
		if usf, _ := m["userSelectedFolders"].([]interface{}); usf != nil {
			for _, x := range usf {
				if fs, _ := x.(string); fs != "" {
					folders[fs] = true
				}
			}
		}
		cli, _ := m["cliSessionId"].(string)
		cwd, _ := m["cwd"].(string)
		if cli == "" || cwd == "" {
			return nil
		}
		pj := filepath.Join(strings.TrimSuffix(p, ".json"), ".claude", "projects")
		if !isDir(pj) {
			return nil
		}
		tot++
		if isFile(filepath.Join(pj, mungeCC(cwd), cli+".jsonl")) {
			ok++
		}
		return nil
	})
	fmiss := 0
	for f := range folders {
		if !isDir(f) {
			fmiss++
		}
	}
	if tot > 0 || len(folders) > 0 {
		logln(cb, "  Verify: conversations resumable: %d/%d", ok, tot)
		if ok < tot {
			logln(cb, "  WARNING: %d conversation(s) may not resume -- try re-running Import over the same package.", tot-ok)
		}
		logln(cb, "  Verify: connected folders present: %d/%d", len(folders)-fmiss, len(folders))
	}
}

func importPkg(pkgDir, mainDest, destOS, destHome, restoreTo, vaultBaseIn string, merge, dryRun bool, cb func(string)) {
	if isFile(pkgDir) && strings.HasSuffix(strings.ToLower(pkgDir), ".zip") {
		tmp, err := os.MkdirTemp("", "cmimp")
		if err != nil {
			logln(cb, "ERROR temp: %v", err)
			return
		}
		defer os.RemoveAll(tmp)
		logln(cb, "Extracting %s ...", pkgDir)
		progN, progPct, progTot = 0, -1, 0
		if zr, e := zip.OpenReader(pkgDir); e == nil {
			for _, zf := range zr.File {
				if !zf.FileInfo().IsDir() {
					progTot++
				}
			}
			zr.Close()
		}
		progCb = cb
		defer func() { progCb = nil }()
		if err := unzip(pkgDir, tmp); err != nil {
			logln(cb, "ERROR unzip: %v", err)
			return
		}
		pkgDir = tmp
		if cb != nil {
			cb("@@S@@Restoring files + rewriting paths\u2026")
			cb("@@P@@65")
		}
	}
	if destOS == "" {
		destOS = osName()
	}
	if destHome == "" {
		destHome, _ = os.UserHomeDir()
	}
	core := filepath.Join(pkgDir, CORE)
	manifest := readJSON(filepath.Join(core, "manifest.json"))
	if manifest == nil {
		logln(cb, "ERROR: no manifest in package")
		return
	}
	srcOS, _ := manifest["source_os"].(string)
	srcHome, _ := manifest["source_home"].(string)
	mainSrc := ""
	if mf, _ := manifest["main_folder"].(map[string]interface{}); mf != nil {
		mainSrc, _ = mf["source_path"].(string)
	}
	getsrc := func(k string) string {
		rts, _ := manifest["roots"].(map[string]interface{})
		if rts == nil {
			return ""
		}
		r, _ := rts[k].(map[string]interface{})
		if r == nil {
			return ""
		}
		s, _ := r["source_path"].(string)
		return s
	}
	srcCowork, srcCC := getsrc("cowork"), getsrc("claude_code")
	natCowork, natCC, natCCJ := defaultRoots(destOS, destHome)
	phys := func(p, sub string) string {
		if restoreTo != "" {
			return filepath.Join(restoreTo, sub)
		}
		return p
	}
	physCowork, physCC, physCCJ := phys(natCowork, "cowork"), phys(natCC, "claude_code"), phys(natCCJ, ".claude.json")
	logln(cb, "IMPORT (%s -> %s)%s", srcOS, destOS, map[bool]string{true: " [merge]", false: " [replace]"}[merge])

	tokens := []*Token{newToken("cowork", srcCowork, srcOS, natCowork), newToken("claude_code", srcCC, srcOS, natCC),
		newToken("main", mainSrc, srcOS, mainDest), newToken("home", srcHome, srcOS, destHome)}
	_ = vaultBaseIn
	type place struct {
		name, dest string
		isMain     bool
	}
	placements := []place{}
	if vm, _ := manifest["vault"].(map[string]interface{}); vm != nil {
		if ents, _ := vm["entries"].([]interface{}); ents != nil {
			for _, e := range ents {
				ent, _ := e.(map[string]interface{})
				if ent == nil {
					continue
				}
				if cp, _ := ent["copied"].(bool); !cp {
					continue
				}
				nm, _ := ent["name"].(string)
				esrc, _ := ent["src"].(string)
				isM, _ := ent["is_main"].(bool)
				if (isM || (mainSrc != "" && norm(esrc) == norm(mainSrc))) && mainDest != "" {
					placements = append(placements, place{nm, mainDest, true})
				} else {
					nd := rewritePathString(esrc, orderTokens(tokens))
					if norm(nd) == norm(esrc) && srcOS != destOS {
						// The folder lived outside every known root (home/main/
						// Claude), so cross-OS its original path is meaningless
						// on this machine -- park it under <home>/ClaudeData/<name>
						// and rewrite references to match.
						nd = joinDest(joinDest(destHome, "ClaudeData"), nm)
					}
					if norm(nd) != norm(esrc) {
						tokens = append(tokens, newToken("data:"+nm, esrc, srcOS, nd))
					}
					placements = append(placements, place{nm, nd, false})
				}
			}
		}
	}
	tokens = orderTokens(tokens)

	if dryRun {
		logln(cb, "DRY RUN (preview) -- nothing will be written.")
		if sc := filepath.Join(core, "cowork"); isDir(sc) {
			skip := []string{}
			kept := 0
			ents, _ := os.ReadDir(sc)
			for _, e := range ents {
				if merge && coworkMergeSkip[e.Name()] {
					skip = append(skip, e.Name())
					continue
				}
				kept += countTree(filepath.Join(sc, e.Name()), map[string]bool{}, []string{})
			}
			logln(cb, "  Would restore Cowork: %d file(s) -> %s", kept, physCowork)
			if merge && len(skip) > 0 {
				logln(cb, "    keeping this machine's: %s", strings.Join(skip, ", "))
			}
			conv := 0
			filepath.WalkDir(filepath.Join(sc, "local-agent-mode-sessions"), func(p string, d os.DirEntry, err error) error {
				if err == nil && !d.IsDir() && strings.HasPrefix(d.Name(), "local_") && strings.HasSuffix(d.Name(), ".json") {
					conv++
				}
				return nil
			})
			logln(cb, "  Conversations in package: %d (transcript folders will be renamed to match this machine)", conv)
		}
		if sc := filepath.Join(core, "claude_code"); isDir(sc) {
			logln(cb, "  Would restore Claude Code (CLI): %d file(s) -> %s", countTree(sc, map[string]bool{}, []string{}), physCC)
		}
		if isFile(filepath.Join(core, "claude_code_home", ".claude.json")) {
			logln(cb, "  Would deep-merge .claude.json (keeps this machine's login) -> %s", physCCJ)
		}
		for _, pl := range placements {
			st := ""
			if isDir(pl.dest) {
				st = " (exists -- will merge)"
			}
			logln(cb, "  Would place data folder %s -> %s%s", pl.name, pl.dest, st)
		}
		if cb != nil {
			cb("@@P@@100")
		}
		announceConnectors(core, cb)
		logln(cb, "PREVIEW complete. Nothing was changed. Click \"Bring everything in\" to apply.")
		return
	}

	if sc := filepath.Join(core, "cowork"); isDir(sc) {
		if merge {
			logln(cb, "  Merging Cowork (keeping login/projects) -> %s", physCowork)
			os.MkdirAll(physCowork, 0755)
			ents, _ := os.ReadDir(sc)
			for _, e := range ents {
				if coworkMergeSkip[e.Name()] {
					continue
				}
				mergeTreeRewrite(filepath.Join(sc, e.Name()), filepath.Join(physCowork, e.Name()), tokens)
			}
		} else {
			backup(physCowork, cb)
			copyTree(sc, physCowork, map[string]bool{"ChromeNativeHost": true}, []string{"buddy-tokens.json"})
			rewriteTree(physCowork, tokens)
		}
	}
	if cb != nil {
		cb("@@P@@80")
	}
	if sc := filepath.Join(core, "claude_code"); isDir(sc) {
		if merge {
			logln(cb, "  Merging Claude Code (keeping credentials) -> %s", physCC)
			os.MkdirAll(physCC, 0755)
			mergeTreeRewrite(sc, physCC, tokens)
		} else {
			backup(physCC, cb)
			copyTree(sc, physCC, map[string]bool{}, []string{})
			rewriteTree(physCC, tokens)
		}
	}
	renameMungedProjectDirs(physCowork, tokens)
	renameMungedProjectDirs(physCC, tokens)
	if cb != nil {
		cb("@@P@@90")
	}
	if sj := filepath.Join(core, "claude_code_home", ".claude.json"); isFile(sj) {
		if merge {
			mergeClaudeJSON(sj, physCCJ, tokens, cb)
		} else {
			backup(physCCJ, cb)
			copyFile(sj, physCCJ)
			rewriteFile(physCCJ, tokens)
		}
	}
	if vs := filepath.Join(pkgDir, EXTRA, VAULTSUB); isDir(vs) && len(placements) > 0 {
		logln(cb, "  Pulling data folders...")
		for _, pl := range placements {
			st := filepath.Join(vs, pl.name)
			if !isDir(st) {
				continue
			}
			if pl.isMain {
				tgt := phys(pl.dest, filepath.Join("main", pl.name))
				if exists(tgt) {
					logln(cb, "    main already at %s -- left untouched", pl.dest)
					continue
				}
				os.MkdirAll(filepath.Dir(tgt), 0755)
				copyTree(st, tgt, map[string]bool{}, []string{})
			} else {
				tgt := phys(pl.dest, filepath.Join("vault", pl.name))
				backup(tgt, cb)
				copyTree(st, tgt, map[string]bool{}, []string{})
			}
		}
	}
	logln(cb, "  Verifying restored setup...")
	verifyImport(physCowork, cb)
	announceConnectors(core, cb)
	if cb != nil {
		cb("@@P@@100")
	}
	logln(cb, "IMPORT complete. Relaunch Claude, sign in again, reinstall plugins.")
}
