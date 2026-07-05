package main

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
)

const maxRewriteBytes = 25 * 1024 * 1024

func logln(cb func(string), f string, a ...interface{}) {
	s := fmt.Sprintf(f, a...)
	fmt.Println(s)
	if cb != nil {
		cb(s)
	}
}

// ----- fs helpers -----
func looksLikeText(path string) bool {
	fi, err := os.Stat(path)
	if err != nil || fi.Size() > maxRewriteBytes {
		return false
	}
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	buf := make([]byte, 65536)
	n, _ := f.Read(buf)
	for _, b := range buf[:n] {
		if b == 0 {
			return false
		}
	}
	return true
}

func matchGlob(name string, globs []string) bool {
	for _, g := range globs {
		if ok, _ := filepath.Match(g, name); ok {
			return true
		}
	}
	return false
}

func copyFile(s, d string) error {
	in, err := os.Open(s)
	if err != nil {
		return err
	}
	defer in.Close()
	os.MkdirAll(filepath.Dir(d), 0755)
	out, err := os.Create(d)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, in)
	out.Close()
	return err
}

func copyTree(src, dst string, exDirs map[string]bool, exGlobs []string) (int, int64, int) {
	files, skipped := 0, 0
	var total int64
	filepath.WalkDir(src, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(src, p)
		if d.IsDir() {
			if rel != "." && (exDirs[d.Name()] || matchGlob(d.Name(), exGlobs)) {
				return filepath.SkipDir
			}
			return nil
		}
		if matchGlob(d.Name(), exGlobs) {
			skipped++
			return nil
		}
		out := filepath.Join(dst, rel)
		if err := copyFile(p, out); err != nil {
			skipped++
			return nil
		}
		files++
		if fi, e := os.Stat(p); e == nil {
			total += fi.Size()
		}
		return nil
	})
	return files, total, skipped
}

func mergeTreeRewrite(staged, live string, tokens []*Token) {
	fi, err := os.Stat(staged)
	if err != nil {
		return
	}
	if !fi.IsDir() {
		copyFile(staged, live)
		if looksLikeText(live) {
			rewriteFile(live, tokens)
		}
		return
	}
	filepath.WalkDir(staged, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(staged, p)
		out := filepath.Join(live, rel)
		if copyFile(p, out) == nil && looksLikeText(out) {
			rewriteFile(out, tokens)
		}
		return nil
	})
}

func nowStamp() string { return time.Now().Format("20060102-150405") }

// backupName is the user-facing zip name: ClaudeBackup_YY-MM-DD_HH-MM-SS
func backupName() string { return "ClaudeBackup_" + time.Now().Format("06-01-02_15-04-05") }

func safeName(s string) string {
	re := regexp.MustCompile("[^A-Za-z0-9._-]+")
	r := strings.Trim(re.ReplaceAllString(s, "_"), "_")
	if r == "" {
		return "item"
	}
	return r
}

func readJSON(p string) map[string]interface{} {
	data, err := os.ReadFile(p)
	if err != nil {
		return nil
	}
	var m map[string]interface{}
	if json.Unmarshal(data, &m) != nil {
		return nil
	}
	return m
}

func writeJSON(p string, v interface{}) {
	os.MkdirAll(filepath.Dir(p), 0755)
	data, _ := json.MarshalIndent(v, "", "  ")
	os.WriteFile(p, data, 0644)
}

func isDir(p string) bool { fi, e := os.Stat(p); return e == nil && fi.IsDir() }

func isFile(p string) bool { fi, e := os.Stat(p); return e == nil && !fi.IsDir() }

func exists(p string) bool { _, e := os.Stat(p); return e == nil }

func osName() string { return runtime.GOOS }

func readRaw(p string) string { b, _ := os.ReadFile(p); return string(b) }

func backup(p string, cb func(string)) {
	if exists(p) {
		b := strings.TrimRight(p, "/\\") + "__backup__" + nowStamp()
		os.Rename(p, b)
		logln(cb, "  backup: %s -> %s", p, b)
	}
}

func zipAddFileBytes(zw *zip.Writer, name string, data []byte) error {
	w, err := zw.Create(filepath.ToSlash(name))
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}

func zipAddFile(zw *zip.Writer, name, src string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return zipAddFileBytes(zw, name, data)
}

var progN, progTot, progPct int

var progCb func(string)

func countTree(src string, exDirs map[string]bool, exGlobs []string) int {
	if !isDir(src) {
		return 0
	}
	n := 0
	filepath.WalkDir(src, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			rel, _ := filepath.Rel(src, p)
			if rel != "." && (exDirs[d.Name()] || matchGlob(d.Name(), exGlobs)) {
				return filepath.SkipDir
			}
			return nil
		}
		if matchGlob(d.Name(), exGlobs) {
			return nil
		}
		n++
		return nil
	})
	return n
}

func dirSizeExcluding(src string, exDirs map[string]bool, exGlobs []string) int64 {
	if !isDir(src) {
		return 0
	}
	var t int64
	filepath.WalkDir(src, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			rel, _ := filepath.Rel(src, p)
			if rel != "." && (exDirs[d.Name()] || matchGlob(d.Name(), exGlobs)) {
				return filepath.SkipDir
			}
			return nil
		}
		if matchGlob(d.Name(), exGlobs) {
			return nil
		}
		if fi, e := os.Stat(p); e == nil {
			t += fi.Size()
		}
		return nil
	})
	return t
}

func zipAddTree(zw *zip.Writer, src, prefix string, exDirs map[string]bool, exGlobs []string) (int, int64) {
	files := 0
	var total int64
	filepath.WalkDir(src, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(src, p)
		if d.IsDir() {
			if rel != "." && (exDirs[d.Name()] || matchGlob(d.Name(), exGlobs)) {
				return filepath.SkipDir
			}
			return nil
		}
		if matchGlob(d.Name(), exGlobs) {
			return nil
		}
		if zipAddFile(zw, prefix+"/"+filepath.ToSlash(rel), p) == nil {
			files++
			if progCb != nil && progTot > 0 {
				progN++
				if pc := progN * 95 / progTot; pc != progPct {
					progPct = pc
					progCb(fmt.Sprintf("@@P@@%d", pc))
				}
			}
			if fi, e := os.Stat(p); e == nil {
				total += fi.Size()
			}
		}
		return nil
	})
	return files, total
}

func unzip(src, dst string) error {
	zr, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer zr.Close()
	cleanDst := filepath.Clean(dst)
	for _, f := range zr.File {
		target := filepath.Join(dst, filepath.FromSlash(f.Name))
		ct := filepath.Clean(target)
		if ct != cleanDst && !strings.HasPrefix(ct, cleanDst+string(os.PathSeparator)) {
			continue
		}
		if f.FileInfo().IsDir() {
			os.MkdirAll(target, 0755)
			continue
		}
		os.MkdirAll(filepath.Dir(target), 0755)
		rc, err := f.Open()
		if err != nil {
			return err
		}
		out, err := os.Create(target)
		if err != nil {
			rc.Close()
			return err
		}
		io.Copy(out, rc)
		out.Close()
		rc.Close()
		if progCb != nil && progTot > 0 {
			progN++
			if pc := progN * 60 / progTot; pc != progPct {
				progPct = pc
				progCb(fmt.Sprintf("@@P@@%d", pc))
			}
		}
	}
	return nil
}

func zipEntries(path string) map[string]bool {
	m := map[string]bool{}
	if zr, err := zip.OpenReader(path); err == nil {
		for _, f := range zr.File {
			m[f.Name] = true
		}
		zr.Close()
	}
	return m
}

func anyPrefix(m map[string]bool, pre string) bool {
	for k := range m {
		if strings.HasPrefix(k, pre) {
			return true
		}
	}
	return false
}
