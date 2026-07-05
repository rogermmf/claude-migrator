package main

import (
	"fmt"
	"os"
	"path/filepath"
)

const VERSION = "3.29.3"

const TOOL = "Finessed Claude Migrator"

func main() {
	args := os.Args[1:]
	if len(args) > 0 && args[0] == "conns" {
		cw, cj := "", ""
		for i := 1; i+1 < len(args); i += 2 {
			if args[i] == "--cowork" {
				cw = args[i+1]
			}
			if args[i] == "--ccjson" {
				cj = args[i+1]
			}
		}
		cmdConns(cw, cj)
		return
	}
	if len(args) > 0 && args[0] == "selftest" {
		os.Exit(selftest())
	}
	if len(args) > 0 && args[0] == "version" {
		fmt.Println(TOOL, VERSION)
		return
	}
	runUI()
}

func cmdConns(coworkRoot, ccjson string) {
	for _, g := range enumerateConnections(coworkRoot, "", ccjson, true) {
		fmt.Printf("[%s] %s\n", g.Scope, g.Name)
		for _, f := range g.Folders {
			fmt.Printf("    - %s (%s, exists=%v)\n", f.Path, mb(f.Bytes), f.Exists)
		}
	}
}

func folderStats(path string) (int, int64, bool) {
	files := 0
	var total int64
	lim := 200000
	capped := false
	filepath.WalkDir(path, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if d.Name() == "node_modules" || d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		files++
		if fi, e := os.Stat(p); e == nil {
			total += fi.Size()
		}
		if files >= lim {
			capped = true
			return filepath.SkipAll
		}
		return nil
	})
	return files, total, capped
}
