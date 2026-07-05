package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func selftest() int {
	fails := 0
	check := func(name string, cond bool) {
		if cond {
			fmt.Println("  PASS " + name)
		} else {
			fmt.Println("  FAIL " + name)
			fails++
		}
	}
	fmt.Println("== token rewriting ==")
	t := newToken("cowork", "C:\\Users\\Example\\AppData\\Roaming\\Claude", "windows", "/Users/macuser/Library/Application Support/Claude")
	check("win->mac text", t.sub("at C:\\Users\\Example\\AppData\\Roaming\\Claude\\local-agent-mode-sessions\\x end", false) == "at /Users/macuser/Library/Application Support/Claude/local-agent-mode-sessions/x end")
	check("win->mac json", t.sub("{\"p\": \"C:\\\\Users\\\\Example\\\\AppData\\\\Roaming\\\\Claude\\\\c.json\"}", true) == "{\"p\": \"/Users/macuser/Library/Application Support/Claude/c.json\"}")
	check("win(fwd)->mac", t.sub("see C:/Users/Example/AppData/Roaming/Claude/skills end", false) == "see /Users/macuser/Library/Application Support/Claude/skills end")
	tm := newToken("main", "C:\\Users\\Example\\Desktop\\Projects", "windows", "/Users/mac/Backup/Projects")
	check("win->mac json spaces", tm.sub("{\"f\":\"C:\\\\Users\\\\Example\\\\Desktop\\\\Projects\\\\My Files\\\\a.txt\"}", true) == "{\"f\":\"/Users/mac/Backup/Projects/My Files/a.txt\"}")
	t2 := newToken("cowork", "/Users/example/Library/Application Support/Claude", "darwin", "C:\\Users\\Bob\\AppData\\Roaming\\Claude")
	check("mac->win text", t2.sub("at /Users/example/Library/Application Support/Claude/s/a stop", false) == "at C:\\Users\\Bob\\AppData\\Roaming\\Claude\\s\\a stop")
	check("mac->win json", t2.sub("{\"p\":\"/Users/example/Library/Application Support/Claude/x\"}", true) == "{\"p\":\"C:\\\\Users\\\\Bob\\\\AppData\\\\Roaming\\\\Claude\\\\x\"}")
	check("munge win", mungeCC(`C:\Users\Example\AppData\Roaming\Claude`) == "C--Users-Example-AppData-Roaming-Claude")
	check("munge misc", mungeCC("/Users/mac user/dir_1") == "-Users-mac-user-dir-1")

	fmt.Println("== exclusions + export/import (merge) ==")
	work, _ := os.MkdirTemp("", "cmtest")
	defer os.RemoveAll(work)
	cowork := filepath.Join(work, "src_cowork")
	os.MkdirAll(filepath.Join(cowork, "local-agent-mode-sessions", "spaces", "s1", "memory"), 0755)
	os.MkdirAll(filepath.Join(cowork, "Cache"), 0755)
	os.MkdirAll(filepath.Join(cowork, "vm_bundles"), 0755)
	os.MkdirAll(filepath.Join(cowork, "local-agent-mode-sessions", "rpm", "plugin_abc", "skills", "myskill"), 0755)
	os.MkdirAll(filepath.Join(cowork, "claude-code-sessions"), 0755)
	os.WriteFile(filepath.Join(cowork, "claude-code-sessions", "local_cc.json"), []byte("{}"), 0644)
	mainD := filepath.Join(work, "MainData")
	os.MkdirAll(filepath.Join(mainD, "Sub"), 0755)
	extra := filepath.Join(work, "ExtraData")
	os.MkdirAll(extra, 0755)
	os.WriteFile(filepath.Join(extra, "data.txt"), []byte("x"), 0644)
	os.WriteFile(filepath.Join(cowork, "claude_desktop_config.json"), []byte("{\"connectedFolders\":[\""+mainD+"\",\""+extra+"\"],\"mcpServers\":{\"github\":{},\"gmail\":{}}}"), 0644)
	os.MkdirAll(filepath.Join(cowork, "ChromeNativeHost"), 0755)
	os.WriteFile(filepath.Join(cowork, "ChromeNativeHost", "pair.json"), []byte("{\"host\":\"old-pc\"}"), 0644)
	os.WriteFile(filepath.Join(cowork, "buddy-tokens.json"), []byte("{\"tok\":\"x\"}"), 0644)
	os.WriteFile(filepath.Join(cowork, "local-agent-mode-sessions", "spaces", "s1", "memory", "MEMORY.md"), []byte("data in "+mainD+"\n"), 0644)
	os.WriteFile(filepath.Join(cowork, "Cache", "j.bin"), []byte("x"), 0644)
	os.WriteFile(filepath.Join(cowork, "vm_bundles", "img.bin"), []byte("x"), 0644)
	os.WriteFile(filepath.Join(cowork, ".credentials.json"), []byte("x"), 0644)
	os.WriteFile(filepath.Join(cowork, "history.jsonl"), []byte("x"), 0644)
	os.WriteFile(filepath.Join(cowork, ".audit-key"), []byte("x"), 0644)
	os.WriteFile(filepath.Join(cowork, "local-agent-mode-sessions", "rpm", "plugin_abc", "skills", "myskill", "SKILL.md"), []byte("---\nname: My Skill\n---\n"), 0644)
	sessDir := filepath.Join(cowork, "local-agent-mode-sessions", "spaces", "s1", "local_ab")
	sessOut := filepath.Join(sessDir, "outputs")
	pd := filepath.Join(sessDir, ".claude", "projects", mungeCC(sessOut))
	os.MkdirAll(pd, 0755)
	os.WriteFile(filepath.Join(pd, "cli-sess-1.jsonl"), []byte("{\"cwd\":\""+sessOut+"\"}\n"), 0644)
	os.WriteFile(filepath.Join(sessDir, "ref.json"), []byte("{\"proj\":\""+mungeCC(sessOut)+"\"}"), 0644)
	os.WriteFile(sessDir+".json", []byte("{\"sessionId\":\"local_ab\",\"cliSessionId\":\"cli-sess-1\",\"cwd\":\""+sessOut+"\",\"title\":\"T\"}"), 0644)

	out := filepath.Join(work, "out")
	curOS := "darwin"
	if osName() == "windows" {
		curOS = "windows"
	}
	pkg := export(cowork, "", "", mainD, "DATA", out, "PKG", curOS, true,
		[]string{mainD, filepath.Join(mainD, "Sub"), extra}, "", false, nil)
	check("export produced a .zip", strings.HasSuffix(pkg, ".zip") && isFile(pkg))
	ents := zipEntries(pkg)
	check("soul cowork present", ents["01_Claude_Core/cowork/claude_desktop_config.json"])
	check("connectors recorded in package", ents["01_Claude_Core/connectors.json"])
	check("chrome pairing excluded from export", !ents["01_Claude_Core/cowork/ChromeNativeHost/pair.json"] && !ents["01_Claude_Core/cowork/buddy-tokens.json"])
	check("Cache excluded", !anyPrefix(ents, "01_Claude_Core/cowork/Cache/"))
	check("vm_bundles excluded", !anyPrefix(ents, "01_Claude_Core/cowork/vm_bundles/"))
	check(".credentials.json excluded", !ents["01_Claude_Core/cowork/.credentials.json"])
	check(".audit-key excluded", !ents["01_Claude_Core/cowork/.audit-key"])
	check("history.jsonl excluded", !ents["01_Claude_Core/cowork/history.jsonl"])
	check("skill backed up", ents["02_Claude_Extra/skills_backup/plugin/My_Skill/SKILL.md"])
	check("vault copied non-main", anyPrefix(ents, "02_Claude_Extra/data_vault/ExtraData/"))
	check("vault skipped sub-of-main", !anyPrefix(ents, "02_Claude_Extra/data_vault/Sub/"))
	check("vault skipped main itself", !anyPrefix(ents, "02_Claude_Extra/data_vault/MainData/"))

	ns := func(s string) string { return sepRe.ReplaceAllString(s, "/") }
	newMain := "D:\\Backup\\MainData"
	rto := filepath.Join(work, "restored")
	impMsgs := []string{}
	importPkg(pkg, newMain, "windows", "C:\\Users\\Bob", rto, "", true, false, func(m string) { impMsgs = append(impMsgs, m) })
	cfg := readRaw(filepath.Join(rto, "cowork", "claude_desktop_config.json"))
	check("import re-pointed main", strings.Contains(ns(cfg), ns(newMain)))
	check("import reconnected connected folder (cross-OS)", strings.Contains(ns(cfg), "C:/Users/Bob") && strings.Contains(cfg, "ExtraData") && !strings.Contains(ns(cfg), ns(extra)))
	check("vault data pulled", isFile(filepath.Join(rto, "vault", "ExtraData", "data.txt")))
	foundMem := false
	filepath.WalkDir(filepath.Join(rto, "cowork", "local-agent-mode-sessions"), func(p string, d os.DirEntry, e error) error {
		if e == nil && !d.IsDir() && d.Name() == "MEMORY.md" {
			foundMem = true
		}
		return nil
	})
	check("memory merged across", foundMem)
	check("code sessions restored (merge)", isFile(filepath.Join(rto, "cowork", "claude-code-sessions", "local_cc.json")))
	expMunged := mungeCC(`C:\Users\Bob\AppData\Roaming\Claude\local-agent-mode-sessions\spaces\s1\local_ab\outputs`)
	check("transcript dir re-munged (resume fix)", isFile(filepath.Join(rto, "cowork", "local-agent-mode-sessions", "spaces", "s1", "local_ab", ".claude", "projects", expMunged, "cli-sess-1.jsonl")))
	check("munged path rewritten inside files", strings.Contains(readRaw(filepath.Join(rto, "cowork", "local-agent-mode-sessions", "spaces", "s1", "local_ab", "ref.json")), expMunged))
	check("verify report: conversation resumable 1/1", strings.Contains(strings.Join(impMsgs, "\n"), "conversations resumable: 1/1"))
	check("connector reconnect popup emitted", strings.Contains(strings.Join(impMsgs, "\n"), "@@POP@@") && strings.Contains(strings.Join(impMsgs, "\n"), "github"))
	check("chrome pairing not restored", !exists(filepath.Join(rto, "cowork", "ChromeNativeHost")) && !exists(filepath.Join(rto, "cowork", "buddy-tokens.json")))

	rtoDry := filepath.Join(work, "restoredDry")
	dryMsgs := []string{}
	importPkg(pkg, newMain, "windows", "C:\\Users\\Bob", rtoDry, "", true, true, func(m string) { dryMsgs = append(dryMsgs, m) })
	dj := strings.Join(dryMsgs, "\n")
	check("dry run writes nothing", !exists(filepath.Join(rtoDry, "cowork")))
	check("dry run reports the plan", strings.Contains(dj, "DRY RUN") && strings.Contains(dj, "Conversations in package"))

	rto2 := filepath.Join(work, "restored2")
	os.MkdirAll(filepath.Join(rto2, "cowork", "Local Storage", "leveldb"), 0755)
	os.WriteFile(filepath.Join(rto2, "cowork", "Local Storage", "leveldb", "LOG"), []byte("x"), 0644)
	importPkg(pkg, newMain, "windows", "C:\\Users\\Bob", rto2, "", true, false, nil)
	check("merge keeps session/login state", isFile(filepath.Join(rto2, "cowork", "Local Storage", "leveldb", "LOG")))
	hasBak := false
	if ents, _ := os.ReadDir(rto2); ents != nil {
		for _, e := range ents {
			if strings.Contains(e.Name(), "__backup__") {
				hasBak = true
			}
		}
	}
	check("merge made no cowork backup", !hasBak)

	rto3 := filepath.Join(work, "restored3")
	importPkg(pkg, "", "windows", "C:\\Users\\Bob", rto3, "", false, false, nil)
	check("replace mode writes cowork", isFile(filepath.Join(rto3, "cowork", "claude_desktop_config.json")))

	if fails == 0 {
		fmt.Println("\nALL PASSED: 0 failed")
	} else {
		fmt.Printf("\nFAILURES: %d failed\n", fails)
	}
	if fails > 0 {
		return 1
	}
	return 0
}
