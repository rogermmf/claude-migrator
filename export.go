package main

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

const CORE = "01_Claude_Core"

const EXTRA = "02_Claude_Extra"

const SKILLSBK = "skills_backup"

const VAULTSUB = "data_vault"

var excludeDirs = map[string]bool{
	"Cache": true, "cache": true, "Code Cache": true, "GPUCache": true, "DawnCache": true,
	"DawnGraphiteCache": true, "DawnWebGPUCache": true, "ShaderCache": true, "GrShaderCache": true,
	"blob_storage": true, "Service Worker": true, "Crashpad": true, "CrashpadMetrics": true,
	"logs": true, "Log": true, "Local Storage": true, "Session Storage": true, "IndexedDB": true,
	"Network": true, "Partitions": true, "component_crx_cache": true, "extensions_crx_cache": true,
	"node_modules": true, ".git": true, ".bin": true, "vm_bundles": true,
	"file-history": true, "debug": true, "telemetry": true, "statsig": true, "ide": true, "tasks": true,
	// machine-bound browser/connector pairing -- never travels (prevents the new
	// machine driving the OLD computer's Chrome through stale pairings)
	"ChromeNativeHost": true,
}

var excludeGlobs = []string{"buddy-tokens.json", "*.lock", "LOCK", "*.ldb", "*.log", "*.tmp", "*.old", ".DS_Store",
	"Thumbs.db", "Singleton*", "*.sock", "lockfile", ".credentials.json", "*.credentials.json",
	".audit-key", "*.audit-key", "history.jsonl"}

var dataExcludeDirs = map[string]bool{".git": true}

var dataExcludeGlobs = []string{".DS_Store", "Thumbs.db", "*.tmp"}

func skillName(p string) string {
	data, _ := os.ReadFile(p)
	re := regexp.MustCompile("(?mi)^\\s*name:\\s*(.+?)\\s*$")
	if m := re.FindStringSubmatch(string(data)); m != nil {
		return strings.Trim(strings.TrimSpace(m[1]), "\"'")
	}
	return filepath.Base(filepath.Dir(p))
}

func findSkills(roots []string) []map[string]string {
	out := []map[string]string{}
	seen := map[string]bool{}
	for _, root := range roots {
		if !isDir(root) {
			continue
		}
		filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			if strings.EqualFold(d.Name(), "SKILL.md") {
				dir := filepath.Dir(p)
				if seen[dir] {
					return nil
				}
				seen[dir] = true
				typ := "skill"
				pl := strings.ToLower(strings.ReplaceAll(dir, "\\", "/"))
				if strings.Contains(pl, "plugin_") {
					typ = "plugin"
				} else if strings.Contains(pl, "/user/") {
					typ = "user"
				} else if strings.Contains(pl, "example") {
					typ = "example"
				}
				out = append(out, map[string]string{"name": skillName(p), "type": typ, "path": dir})
			}
			return nil
		})
	}
	return out
}

func export(cowork, claudeCode, ccjson, mainPath, mainLabel, outDir, name, sourceOS string,
	includeSkills bool, vaultFolders []string, vaultDest string, includeMain bool, cb func(string)) string {
	home, _ := os.UserHomeDir()
	host, _ := os.Hostname()
	if name == "" {
		name = backupName()
	}
	os.MkdirAll(outDir, 0755)
	zipPath := filepath.Join(outDir, name+".zip")
	f, err := os.Create(zipPath)
	if err != nil {
		logln(cb, "ERROR creating zip: %v", err)
		return ""
	}
	zw := zip.NewWriter(f)
	logln(cb, "EXPORT -> %s", zipPath)
	progCb = cb
	progN = 0
	progPct = -1
	progTot = 0
	if isDir(cowork) {
		progTot += countTree(cowork, excludeDirs, excludeGlobs)
	}
	if isDir(claudeCode) {
		progTot += countTree(claudeCode, excludeDirs, excludeGlobs)
	}
	if includeSkills {
		for _, sk := range findSkills([]string{cowork, claudeCode}) {
			progTot += countTree(sk["path"], dataExcludeDirs, dataExcludeGlobs)
		}
	}
	for _, src := range vaultFolders {
		sameMain := mainPath != "" && norm(src) == norm(mainPath)
		strictIn := mainPath != "" && isSub(src, mainPath) && !sameMain
		if isDir(src) && !strictIn && !(sameMain && !includeMain) {
			progTot += countTree(src, dataExcludeDirs, dataExcludeGlobs)
		}
	}
	defer func() { progCb = nil }()
	roots := map[string]interface{}{}
	if isDir(cowork) {
		fl, b := zipAddTree(zw, cowork, "01_Claude_Core/cowork", excludeDirs, excludeGlobs)
		roots["cowork"] = map[string]interface{}{"present": true, "source_path": cowork, "files": fl, "bytes": b}
		logln(cb, "  [Core] Cowork: %d files, %.1f MB", fl, float64(b)/1e6)
	} else {
		roots["cowork"] = map[string]interface{}{"present": false, "source_path": cowork}
	}
	if isDir(claudeCode) {
		fl, _ := zipAddTree(zw, claudeCode, "01_Claude_Core/claude_code", excludeDirs, excludeGlobs)
		roots["claude_code"] = map[string]interface{}{"present": true, "source_path": claudeCode, "files": fl}
	} else {
		roots["claude_code"] = map[string]interface{}{"present": false, "source_path": claudeCode}
	}
	if isFile(ccjson) {
		zipAddFile(zw, "01_Claude_Core/claude_code_home/.claude.json", ccjson)
		roots["claude_code_json"] = map[string]interface{}{"present": true, "source_path": ccjson}
	} else {
		roots["claude_code_json"] = map[string]interface{}{"present": false, "source_path": ccjson}
	}
	if includeSkills {
		used := map[string]bool{}
		n := 0
		for _, sk := range findSkills([]string{cowork, claudeCode}) {
			base := safeName(sk["name"])
			nm := base
			i := 2
			for used[sk["type"]+"/"+nm] {
				nm = base + "_" + fmt.Sprint(i)
				i++
			}
			used[sk["type"]+"/"+nm] = true
			zipAddTree(zw, sk["path"], "02_Claude_Extra/skills_backup/"+sk["type"]+"/"+nm, dataExcludeDirs, dataExcludeGlobs)
			n++
		}
		if n > 0 {
			logln(cb, "  [Extra] Backed up %d skills", n)
		}
	}
	vaultEntries := []map[string]interface{}{}
	if len(vaultFolders) > 0 {
		logln(cb, "  [Extra] Vaulting data folder(s)...")
		used := map[string]bool{}
		for _, src := range vaultFolders {
			sameMain := mainPath != "" && norm(src) == norm(mainPath)
			strictIn := mainPath != "" && isSub(src, mainPath) && !sameMain
			base := safeName(filepath.Base(strings.TrimRight(src, "/\\")))
			nm := base
			i := 2
			for used[nm] {
				nm = base + "_" + fmt.Sprint(i)
				i++
			}
			used[nm] = true
			ent := map[string]interface{}{"src": src, "name": nm, "inside_main": strictIn || sameMain, "is_main": sameMain, "copied": false}
			if strictIn {
				logln(cb, "    skip (inside main): %s", src)
			} else if sameMain && !includeMain {
				logln(cb, "    skip main folder -- migrate yourself: %s", src)
			} else if isDir(src) {
				fl, b := zipAddTree(zw, src, "02_Claude_Extra/data_vault/"+nm, dataExcludeDirs, dataExcludeGlobs)
				ent["copied"] = true
				ent["files"] = fl
				ent["bytes"] = b
				logln(cb, "    vaulted %s (%d files)", src, fl)
			} else {
				logln(cb, "    not found: %s", src)
			}
			vaultEntries = append(vaultEntries, ent)
		}
	}
	manifest := map[string]interface{}{
		"tool": TOOL, "version": VERSION, "schema": 3, "created_utc": time.Now().UTC().Format(time.RFC3339),
		"source_os": sourceOS, "source_hostname": host, "source_home": home, "roots": roots,
		"main_folder": map[string]interface{}{"label": mainLabel, "source_path": mainPath},
		"vault":       map[string]interface{}{"built": len(vaultEntries) > 0, "external": false, "entries": vaultEntries},
	}
	mb, _ := json.MarshalIndent(manifest, "", "  ")
	zipAddFileBytes(zw, "01_Claude_Core/manifest.json", mb)
	ob, _ := json.MarshalIndent(map[string]interface{}{"tool": TOOL, "version": VERSION, "source_os": sourceOS,
		"include_skills": includeSkills, "data_backup": len(vaultFolders) > 0}, "", "  ")
	zipAddFileBytes(zw, "migration_options.json", ob)
	zipAddFileBytes(zw, "README_RESTORE.txt", []byte("Open Claude Migrator on the new computer, choose Import, pick this .zip.\nThen relaunch Claude, sign in again, and reinstall plugins.\n"))
	zipAddFileBytes(zw, "connections.html", []byte(connectionsHTML(enumerateConnections(cowork, claudeCode, ccjson, true), findSkills([]string{cowork, claudeCode}), enumerateConnectors(cowork, claudeCode, ccjson), host)))
	if cl := enumerateConnectors(cowork, claudeCode, ccjson); len(cl) > 0 {
		sort.Strings(cl)
		cb2, _ := json.Marshal(map[string]interface{}{"connectors": cl})
		zipAddFileBytes(zw, CORE+"/connectors.json", cb2)
		logln(cb, "  Connectors recorded: %d (machine-bound -- you'll reconnect them after import)", len(cl))
	}
	zw.Close()
	f.Close()
	if cb != nil {
		cb("@@S@@Verifying archive\u2026")
		cb("@@P@@97")
	}
	if zr, err := zip.OpenReader(zipPath); err == nil {
		n := len(zr.File)
		zr.Close()
		logln(cb, "  verified zip: %d entries", n)
	} else {
		logln(cb, "  WARNING: zip verify failed: %v", err)
	}
	if cb != nil {
		cb("@@P@@100")
	}
	logln(cb, "EXPORT done -> %s", zipPath)
	return zipPath
}
