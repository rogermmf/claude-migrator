package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
	"time"
)

func writeJSONHTTP(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func openBrowser(url string) {
	switch osName() {
	case "darwin":
		exec.Command("open", url).Start()
	case "windows":
		exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	default:
		exec.Command("xdg-open", url).Start()
	}
}

// Lifecycle: the app has no window of its own -- the browser tab IS the UI.
// The page heartbeats /api/ping every 3s; once armed (first ping seen), the
// watchdog exits the process ~15s after the last tab closes, unless an
// export/import is running. If no browser ever connects, it exits after 5 min.
var lastPing atomic.Int64
var activeOps atomic.Int32

func watchdog() {
	start := time.Now()
	for {
		time.Sleep(5 * time.Second)
		lp := lastPing.Load()
		if activeOps.Load() > 0 {
			continue
		}
		if lp == 0 {
			if time.Since(start) > 5*time.Minute {
				fmt.Println("No browser connected for 5 minutes -- exiting.")
				os.Exit(0)
			}
			continue
		}
		if time.Since(time.Unix(lp, 0)) > 15*time.Second {
			fmt.Println("Browser tab closed -- exiting.")
			os.Exit(0)
		}
	}
}

func runUI() {
	home, _ := os.UserHomeDir()
	osn := osName()
	cowork, cc, ccj := defaultRoots(osn, home)
	if osn == "windows" && !isDir(cowork) {
		alt := filepath.Join(home, "AppData", "Local", "Packages", "Claude_pzs8sxrjxfjjc", "LocalCache", "Roaming", "Claude")
		if isDir(alt) {
			cowork = alt
		}
	}
	if osn == "windows" && (!isDir(cowork) || !isDir(cc)) {
		u := filepath.Base(home)
		for c := 'D'; c <= 'Z'; c++ {
			root := string(c) + ":" + string(os.PathSeparator)
			if !isDir(cowork) {
				for _, cand := range []string{filepath.Join(root, "Users", u, "AppData", "Roaming", "Claude"), filepath.Join(root, "Users", u, "AppData", "Local", "Packages", "Claude_pzs8sxrjxfjjc", "LocalCache", "Roaming", "Claude")} {
					if isDir(cand) {
						cowork = cand
						break
					}
				}
			}
			if !isDir(cc) {
				if cand := filepath.Join(root, "Users", u, ".claude"); isDir(cand) {
					cc = cand
					ccj = filepath.Join(root, "Users", u, ".claude.json")
				}
			}
		}
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/assets/finessed.png", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Write(finessedPNG)
	})
	mux.HandleFunc("/assets/unicornus.png", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Write(unicornusPNG)
	})
	mux.HandleFunc("/assets/crab.png", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Write(crabPNG)
	})
	mux.HandleFunc("/assets/walk_a.png", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Write(walkA)
	})
	mux.HandleFunc("/assets/walk_b.png", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Write(walkB)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		io.WriteString(w, indexHTML)
	})
	mux.HandleFunc("/api/ping", func(w http.ResponseWriter, r *http.Request) {
		lastPing.Store(time.Now().Unix())
		io.WriteString(w, "ok")
	})
	mux.HandleFunc("/api/quit", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "bye")
		go func() { time.Sleep(300 * time.Millisecond); os.Exit(0) }()
	})
	mux.HandleFunc("/favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Write(crabPNG)
	})
	mux.HandleFunc("/api/defaults", func(w http.ResponseWriter, r *http.Request) {
		writeJSONHTTP(w, map[string]interface{}{"os": osn, "cowork": cowork, "claudeCode": cc,
			"home": home, "desktop": filepath.Join(home, "Desktop"), "version": VERSION})
	})
	mux.HandleFunc("/api/ls", func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Query().Get("path")
		if p == "" {
			p = home
		}
		dirs := []string{}
		if ents, err := os.ReadDir(p); err == nil {
			for _, e := range ents {
				if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
					dirs = append(dirs, e.Name())
				}
			}
		}
		sort.Strings(dirs)
		files := []string{}
		if ents, err := os.ReadDir(p); err == nil {
			for _, e := range ents {
				if !e.IsDir() && strings.HasSuffix(strings.ToLower(e.Name()), ".zip") {
					files = append(files, e.Name())
				}
			}
		}
		sort.Strings(files)
		writeJSONHTTP(w, map[string]interface{}{"path": p, "parent": filepath.Dir(p),
			"sep": string(os.PathSeparator), "dirs": dirs, "files": files})
	})
	mux.HandleFunc("/api/drives", func(w http.ResponseWriter, r *http.Request) {
		writeJSONHTTP(w, map[string]interface{}{"drives": driveRoots(), "sep": string(os.PathSeparator)})
	})
	mux.HandleFunc("/api/scan", func(w http.ResponseWriter, r *http.Request) {
		projects := enumerateConnections(cowork, cc, ccj, true)
		soul := dirSizeExcluding(cowork, excludeDirs, excludeGlobs) + dirSizeExcluding(cc, excludeDirs, excludeGlobs)
		writeJSONHTTP(w, map[string]interface{}{"projects": projects, "soulBytes": soul, "warnings": probeLayout(cowork)})
	})
	mux.HandleFunc("/api/export", func(w http.ResponseWriter, r *http.Request) {
		activeOps.Add(1)
		defer activeOps.Add(-1)
		var b struct {
			Cowork, ClaudeCode, OutDir, Mode string
			IncludeSkills                    bool
			Folders                          []string
		}
		json.NewDecoder(r.Body).Decode(&b)
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		fl, _ := w.(http.Flusher)
		cb := func(s string) {
			io.WriteString(w, s+"\n")
			if fl != nil {
				fl.Flush()
			}
		}
		defer func() {
			if rec := recover(); rec != nil {
				cb(fmt.Sprintf("ERROR: %v", rec))
			}
		}()
		if b.OutDir == "" {
			cb("Please choose where to save the package.")
			return
		}
		folders := []string{}
		switch b.Mode {
		case "all":
			folders = allConnectedFolders(b.Cowork, b.ClaudeCode, ccj)
			cb(fmt.Sprintf("  including all %d connected folder(s)", len(folders)))
		case "partial":
			folders = b.Folders
			cb(fmt.Sprintf("  including %d selected folder(s)", len(folders)))
		default:
			cb("  core only (no connected data folders)")
		}
		for _, wmsg := range probeLayout(b.Cowork) {
			cb("  WARNING: " + wmsg)
		}
		export(b.Cowork, b.ClaudeCode, ccj, "", "MAIN", b.OutDir, "", osn, b.IncludeSkills, folders, "", false, cb)
	})
	mux.HandleFunc("/api/import", func(w http.ResponseWriter, r *http.Request) {
		activeOps.Add(1)
		defer activeOps.Add(-1)
		var b struct {
			PkgDir, VaultBase string
			Merge, DryRun     bool
		}
		json.NewDecoder(r.Body).Decode(&b)
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		fl, _ := w.(http.Flusher)
		cb := func(s string) {
			io.WriteString(w, s+"\n")
			if fl != nil {
				fl.Flush()
			}
		}
		defer func() {
			if rec := recover(); rec != nil {
				cb(fmt.Sprintf("ERROR: %v", rec))
			}
		}()
		if b.PkgDir == "" {
			cb("Please pick the package folder.")
			return
		}
		importPkg(b.PkgDir, "", osn, home, "", b.VaultBase, b.Merge, b.DryRun, cb)
	})
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		fmt.Println("cannot start:", err)
		return
	}
	url := "http://" + ln.Addr().String() + "/"
	fmt.Println(TOOL, VERSION, "running at", url)
	go watchdog()
	openBrowser(url)
	http.Serve(ln, mux)
}

const indexHTML = `<!doctype html><html><head><meta charset="utf-8"><title>Claude Migrator</title><link rel="icon" type="image/png" href="/assets/crab.png">
<style>
body{font-family:-apple-system,Segoe UI,Arial,sans-serif;margin:0;background:#1d1f23;color:#e6e6e6}
.wrap{max-width:860px;margin:0 auto;padding:18px 18px 26px}
.head{display:flex;align-items:center;gap:13px;margin-bottom:2px}
.crab{width:56px;height:auto;image-rendering:pixelated}.mascot{position:relative;display:inline-block;width:112px;height:118px;image-rendering:pixelated}.mascot img{position:absolute;left:0;bottom:0;width:112px;height:auto;image-rendering:pixelated}.mascot .fA{animation:hop 1.2s steps(2) infinite,fadeA 1.2s step-end infinite}.mascot .fB{animation:hop 1.2s steps(2) infinite,fadeB 1.2s step-end infinite}@keyframes fadeA{0%{opacity:1}50%{opacity:0}100%{opacity:1}}@keyframes fadeB{0%{opacity:0}50%{opacity:1}100%{opacity:0}}@keyframes hop{0%{transform:scaleX(-1) translateY(0)}50%{transform:scaleX(-1) translateY(-4px)}100%{transform:scaleX(-1) translateY(0)}}
h1{font-size:21px;margin:0}.sub{color:#9aa0a6;font-size:13px;margin:3px 0 14px}
.tabs{display:flex;gap:8px;margin-bottom:6px}
.tab{padding:8px 16px;border-radius:9px;background:#2a2d31;cursor:pointer;font-weight:600;font-size:13px}
.tab.active{background:#d97757;color:#1d1f23}
.intro{font-size:13px;color:#c2c7cd;background:#23262b;border:1px solid #2f333a;border-radius:8px;padding:9px 11px;margin:11px 0}
.row{display:flex;align-items:center;margin:11px 0 2px;gap:8px}
.row label{width:180px;font-size:13px;color:#e6e6e6;font-weight:600}
.row input[type=text]{flex:1;padding:8px;border-radius:6px;border:1px solid #3a3d42;background:#15171a;color:#e6e6e6}
.help{font-size:12px;color:#9aa0a6;margin:0 0 4px 188px;line-height:1.5}
.ex{color:#d9a07a}
button{padding:9px 16px;border:0;border-radius:8px;background:#d97757;color:#1d1f23;font-weight:700;cursor:pointer}
button.sec{background:#2a2d31;color:#cfd3d7;font-weight:600}
.chk{margin:10px 0 2px;font-size:14px}.chkhelp{font-size:12px;color:#9aa0a6;margin:0 0 4px 24px;line-height:1.5}.rad{font-size:13px;line-height:2}.tscope{margin-top:8px;font-weight:700;color:#cfd3d7;border-bottom:1px solid #333;padding-bottom:2px}.tproj{margin:6px 0 2px;font-weight:600;color:#d97757}.tfold{display:flex;gap:6px;align-items:center;margin-left:16px;padding:2px 0}.tfold span.nm{word-break:break-all}.tfold .sz{margin-left:auto;color:#9aa0a6;font-size:12px;white-space:nowrap}
.hide{display:none}
pre{background:#0e1013;border:1px solid #2a2d31;border-radius:8px;padding:10px;height:190px;overflow:auto;font-size:12px;white-space:pre-wrap;margin-top:12px}
textarea{width:100%;height:62px;background:#15171a;color:#e6e6e6;border:1px solid #3a3d42;border-radius:6px;padding:7px}
#picker{position:fixed;inset:0;background:rgba(0,0,0,.5);display:none;align-items:center;justify-content:center}
#pbox{background:#1d1f23;border:1px solid #3a3d42;border-radius:10px;width:600px;max-height:74vh;display:flex;flex-direction:column}
#plist{overflow:auto;padding:6px}.pitem{padding:7px 10px;border-radius:6px;cursor:pointer;font-size:13px}.pitem:hover{background:#2a2d31}
.topbar{position:sticky;top:0;z-index:50;background:#1d1f23;border-bottom:1px solid #2a2d31;color:#7e848b;padding:10px 0}.tbinner{position:relative;max-width:860px;margin:0 auto;padding:0 18px;display:grid;grid-template-columns:1fr auto 1fr;align-items:center;column-gap:20px}.topbar .hcenter{text-align:center;justify-self:center}.topbar .mascot{justify-self:center}.topbar .powered{display:flex;flex-direction:column;align-items:center;gap:4px;justify-self:start}.topbar h1{font-size:20px;margin:0;color:#e6e6e6}.quitbtn{position:absolute;top:10px;right:16px;color:#7e848b;font-size:12px;text-decoration:none;border:1px solid #2a2d31;border-radius:6px;padding:3px 8px}.quitbtn:hover{color:#e6e6e6;border-color:#555}.topbar .ver{font-size:11px;color:#7e848b;margin-top:2px;letter-spacing:.3px}.topbar .sub{color:#9aa0a6;font-size:13px;margin:2px 0 6px}.topbar .cred{font-size:12px;color:#7e848b;line-height:1.55}.topbar .hlogos{display:flex;align-items:center;gap:14px;justify-content:center}.topbar .hlogos a{display:inline-flex;align-items:center}
.topbar .hlogos img{height:75px;width:auto;opacity:.95}.topbar .hlogos .ic{height:69px}.gated{opacity:.4;pointer-events:none;filter:grayscale(.35)}.scanbtn{background:#d97757}.bar{height:14px;background:#2a2d31;border-radius:8px;overflow:hidden;margin:10px 0;display:none}.bar>i{display:block;height:100%;width:0;background:#d97757;transition:width .2s}.tnote{margin-left:16px;color:#8a9099;font-size:12px;font-style:italic;padding:1px 0}.scanline{font-size:12px;color:#9aa0a6}.sizebadge{font-weight:700;font-size:13px;color:#e6e6e6}.barrow{display:flex;align-items:center;gap:10px}.barrow .bar{flex:1}.pctlbl{font-size:12px;font-weight:700;color:#e6e6e6;min-width:40px;text-align:right;display:none}.stageline{font-size:12px;color:#9aa0a6;font-style:italic;display:none;margin:0 0 8px}.warnline{color:#e0a458;font-size:12px;margin:4px 0}.note{color:#9aa0a6;font-size:12px;margin:8px 0;line-height:1.5}
</style></head><body>
<div class="topbar"><div class="tbinner">
 <span class="mascot"><img class="fA" src="/assets/walk_a.png" alt=""><img class="fB" src="/assets/walk_b.png" alt=""></span>
 <a class="quitbtn" href="#" onclick="quitApp();return false" title="Close Claude Migrator">&#10005; Quit</a>
 <div class="hcenter">
  <h1>Claude Migrator</h1>
  <div class="ver" id="ver"></div>
  <div class="sub">Move Claude between computers (Windows and Mac). One file, no setup.</div>
  <div class="cred">Open source &mdash; for Claude</div>
  <div class="cred">built by: Roger Martinez</div>
  <div class="cred"><a href="https://github.com/rogermmf" target="_blank" rel="noopener" style="color:#9aa0a6;text-decoration:none">github.com/rogermmf</a></div>
 </div>
 <div class="powered">
  <div class="cred">Powered by:</div>
  <div class="hlogos">
   <a href="https://unicornus.ai" target="_blank" rel="noopener" title="Unicornus.ai"><img class="ic" src="/assets/unicornus.png" alt="Unicornus.ai"></a>
   <a href="https://thefinessedhub.com" target="_blank" rel="noopener" title="The Finessed Hub"><img src="/assets/finessed.png" alt="The Finessed Hub"></a>
  </div>
 </div>
 </div>
</div>
<div class="wrap">
<div class="tabs"><div class="tab active" id="tab-export" onclick="show('export')">Export &mdash; old computer</div><div class="tab" id="tab-import" onclick="show('import')">Import &mdash; new computer</div></div>

<div id="panel-export">
 <div class="intro">Run this on the computer you are LEAVING. First scan your system, then pack Claude into a single .zip you carry to the other computer.</div>
 <div class="row" style="margin-top:4px"><button id="e_scan" class="scanbtn" onclick="scanConns()">Scan my system</button><span id="e_scanstatus" class="scanline">Not scanned yet &mdash; start here.</span></div><div id="e_warn"></div>
 <div class="note">Your claude.ai chats and <b>Claude Design</b> projects live in your Claude account and are backed up in Claude&rsquo;s cloud automatically &mdash; this tool backs up what lives on <i>this computer</i>. Connector logins are machine-bound and are <b>not</b> migrated (prevents cross-machine mixing); after import you&rsquo;ll get a list of connectors to reconnect.</div>
 <div class="chkhelp" style="margin-left:0">This only <b>reads</b> your computer to find your projects, their connected folders and total size. It never changes, moves or deletes any file. Everything below stays locked until the scan finishes.</div>
 <div id="e_gate" class="gated">
  <div class="row"><label>Cowork/Desktop folder</label><input type="text" id="e_cowork"><button class="sec" onclick="browse('e_cowork')">Browse</button></div>
  <div class="help">This computer's Claude Desktop/Cowork data. Auto-detected &mdash; leave it as-is unless you know it is wrong.</div>
  <div class="row"><label>Claude Code folder</label><input type="text" id="e_cc"><button class="sec" onclick="browse('e_cc')">Browse</button></div>
  <div class="help">This computer's Claude Code (terminal) data. Auto-detected.</div>
  <div class="row"><label>Save package to</label><input type="text" id="e_out"><button class="sec" onclick="browse('e_out')">Browse</button></div>
  <div class="help">Folder where the export file is written. Tip: your Desktop.</div>
  <div class="chkhelp" style="margin-left:0">The core &mdash; settings, memory, skills, and every project's context + conversations &mdash; is backed up by default once you run the app.</div>
  <label class="chk" style="cursor:pointer"><input type="checkbox" id="e_conn" onchange="connToggle()"> <b>Back up connected folders + files too</b></label>
  <div class="chkhelp">Also copy the actual data folders your projects point to, so they travel with you and reconnect on the other computer.</div>
  <div id="e_connopts" class="hide" style="margin:6px 0 2px 24px">
   <select id="e_connmode" onchange="connModeChanged()" style="background:#2a2d31;color:#e6e6e6;border:0;border-radius:7px;padding:7px 10px;font-size:13px;font-weight:600">
    <option value="all">All of it</option>
    <option value="partial">Choose per project</option>
   </select>
  </div>
  <div id="e_treebox" class="hide" style="max-height:320px;overflow:auto;background:#1b1d21;border-radius:8px;padding:8px;font-size:13px;margin:8px 0">Run the scan to see your projects and their connected folders here.</div>
  <div class="barrow"><div class="bar" id="e_bar"><i id="e_barfill"></i></div><span class="pctlbl" id="e_bar_pct"></span></div><div class="stageline" id="e_bar_stage"></div>
  <div class="row" style="margin-top:12px;align-items:center"><button onclick="doExport()">Create the .zip</button><span id="e_total" class="sizebadge" style="margin-left:14px"></span></div>
 </div>
</div>

<div id="panel-import" class="hide">
 <div class="intro">Run this on the computer you are MOVING TO. Install Claude and sign in, then point this at the .zip. Your Cowork and Code projects come back and reconnect to their folders automatically.</div>
 <div class="row"><label>Package (.zip)</label><input type="text" id="i_pkg"><button class="sec" onclick="browse('i_pkg','zip')">Browse</button></div>
 <div class="help"><b>What it is:</b> the .zip you made with Export on the other computer. Click <b>Browse</b>, open the folder where you saved it, and click the file &mdash; it appears as <span class="ex">(zip) ClaudeMigration_&hellip;.zip</span>.</div>
 <div class="help">Everything in the package is brought in exactly as it was &mdash; your Cowork &amp; Code projects, memory, skills, and every connected folder is restored to its original place (translated for this computer) and <b>auto-reconnected</b>. Nothing else to choose.</div>
 <div class="chk"><input type="checkbox" id="i_merge" checked> Safe merge (recommended)</div>
 <div class="note">Connectors are reset on import &mdash; a list of what to reconnect will pop up.</div>
 <div class="chkhelp"><b>ON</b> &mdash; adds your memory, skills and past sessions to this computer's Claude and never touches your current sign-in or your Projects.<br><b>OFF</b> &mdash; completely overwrite this computer's Claude with the package (advanced; you will be signed out and have to log back in).</div>
 <div class="barrow"><div class="bar" id="i_bar"><i id="i_barfill"></i></div><span class="pctlbl" id="i_bar_pct"></span></div><div class="stageline" id="i_bar_stage"></div>
 <div class="row" style="margin-top:14px"><button class="sec" onclick="doImport(true)">Preview (dry run)</button><button onclick="doImport(false)">Bring everything in</button></div>
 <div class="chkhelp" style="margin-left:0;margin-top:8px">When it finishes: restart Claude, sign in again, and reinstall your plugins.</div>
</div>

<pre id="log"></pre>
</div>
<div id="picker"><div id="pbox"><div class="row" style="padding:10px"><b id="pcur" style="flex:1;font-size:12px"></b><button class="sec" onclick="showDrives()">Drives</button><button class="sec" onclick="closePicker()">Cancel</button><button onclick="choose()">Use this folder</button></div><div id="plist"></div></div></div>
<script>
setInterval(function(){fetch("/api/ping").catch(function(){})},3000);
function showConnPopup(list){var ov=document.createElement("div");ov.style.cssText="position:fixed;inset:0;background:rgba(0,0,0,.55);z-index:99;display:flex;align-items:center;justify-content:center";var bx=document.createElement("div");bx.style.cssText="background:#26282c;border:1px solid #3a3d42;border-radius:10px;max-width:460px;padding:22px 26px;color:#e6e6e6;font-size:14px;line-height:1.55";bx.innerHTML="<b>Reconnect your connectors</b><br><br>Connector logins are tied to each computer (and its own browser), so they were <b>not</b> carried over &mdash; otherwise this machine could keep driving your old computer&rsquo;s browser.<br><br>Reconnect these in Claude on this machine:<br><br>"+list.map(function(c){return "&bull; "+c;}).join("<br>")+"<br><br>";var ok=document.createElement("button");ok.textContent="Got it";ok.onclick=function(){document.body.removeChild(ov);};bx.appendChild(ok);ov.appendChild(bx);document.body.appendChild(ov);}
function quitApp(){fetch("/api/quit").finally(function(){document.body.innerHTML='<div style="font-family:sans-serif;color:#9aa0a6;text-align:center;margin-top:120px">Claude Migrator closed. You can close this tab.</div>';});}
var dflt={};
function show(t){document.getElementById("panel-export").className=t=="export"?"":"hide";document.getElementById("panel-import").className=t=="import"?"":"hide";document.getElementById("tab-export").className="tab"+(t=="export"?" active":"");document.getElementById("tab-import").className="tab"+(t=="import"?" active":"");}
function toggleData(){document.getElementById("e_datawrap").className=document.getElementById("e_data").checked?"":"hide";}
function gv(id){return document.getElementById(id).value.trim();}
async function load(){var r=await fetch("/api/defaults");dflt=await r.json();document.getElementById("e_cowork").value=dflt.cowork||"";document.getElementById("e_cc").value=dflt.claudeCode||"";document.getElementById("e_out").value=dflt.desktop||dflt.home||"";var ve=document.getElementById("ver");if(ve)ve.textContent="v"+(dflt.version||"");}
load();
var pTarget=null,pPath="",pMode="dir";
function browse(id,mode){pTarget=id;pMode=mode||"dir";openAt(gv(id)||dflt.home||"/");document.getElementById("picker").style.display="flex";}
function closePicker(){document.getElementById("picker").style.display="none";}
function choose(){if(pTarget)document.getElementById(pTarget).value=pPath;closePicker();}
async function openAt(p){var r=await fetch("/api/ls?path="+encodeURIComponent(p));var j=await r.json();pPath=j.path;document.getElementById("pcur").textContent=j.path;var list=document.getElementById("plist");list.innerHTML="";var up=document.createElement("div");up.className="pitem";up.textContent=".. (up)";up.onclick=function(){openAt(j.parent);};list.appendChild(up);j.dirs.forEach(function(d){var el=document.createElement("div");el.className="pitem";el.textContent="[dir] "+d;el.onclick=function(){openAt(j.path.replace(/[\/\\]+$/,"")+(j.sep||"/")+d);};list.appendChild(el);});if(pMode=="zip"&&j.files){j.files.forEach(function(fn){var fe=document.createElement("div");fe.className="pitem";fe.textContent="(zip) "+fn;fe.onclick=function(){if(pTarget)document.getElementById(pTarget).value=j.path.replace(/[\/\\]+$/,"")+(j.sep||"/")+fn;closePicker();};list.appendChild(fe);});}}
async function stream(url,body){var L=document.getElementById("log");L.textContent="Working...";var r=await fetch(url,{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify(body)});var rd=r.body.getReader();var dec=new TextDecoder();L.textContent="";while(true){var x=await rd.read();if(x.done)break;L.textContent+=dec.decode(x.value);L.scrollTop=L.scrollHeight;}}
var SOUL=0, ALLSIZE=0;
function fmtBytes(n){n=parseInt(n||0);if(!n)return "0 B";var u=["B","KB","MB","GB","TB"];var i=Math.floor(Math.log(n)/Math.log(1024));return (n/Math.pow(1024,i)).toFixed(i?1:0)+" "+u[i];}
function connToggle(){var on=document.getElementById("e_conn").checked;document.getElementById("e_connopts").className=on?"":"hide";connModeChanged();}
function connModeChanged(){var on=document.getElementById("e_conn").checked;var mode=document.getElementById("e_connmode").value;var showTree=on&&mode=="partial";document.getElementById("e_treebox").className=showTree?"":"hide";if(showTree){document.querySelectorAll(".tfold input").forEach(function(c){c.disabled=(c.getAttribute("data-exists")!="1");});}recalcTotal();}
function recalcTotal(){var el=document.getElementById("e_total");if(!el)return;var on=document.getElementById("e_conn").checked;var mode=document.getElementById("e_connmode").value;var t=SOUL,note="Claude core only";if(on&&mode=="all"){t=SOUL+ALLSIZE;note="core + all connected folders";}else if(on){var add=0,k=0;document.querySelectorAll(".tfold input:checked").forEach(function(c){add+=parseInt(c.getAttribute("data-bytes")||"0");k++;});t=SOUL+add;note="core + "+k+" folder"+(k==1?"":"s");}el.textContent="Backup size: "+fmtBytes(t)+" · "+note;}
async function scanConns(){var st=document.getElementById("e_scanstatus");var sb=document.getElementById("e_scan");st.textContent="Scanning… large folders take a moment.";sb.disabled=true;var box=document.getElementById("e_treebox");box.textContent="Scanning…";try{var r=await fetch("/api/scan");var j=await r.json();SOUL=parseInt(j.soulBytes||0);var proj=j.projects||[];box.innerHTML="";var scopes={};proj.forEach(function(p){(scopes[p.scope]=scopes[p.scope]||[]).push(p);});var nf=0,np=0,uniq={};Object.keys(scopes).forEach(function(sc){var h=document.createElement("div");h.className="tscope";h.textContent=sc+" projects";box.appendChild(h);scopes[sc].forEach(function(p){np++;var ph=document.createElement("div");ph.className="tproj";ph.textContent=p.name;box.appendChild(ph);var fs=p.folders||[];if(!fs.length){var nn=document.createElement("div");nn.className="tnote";nn.textContent="no connected folder — context + conversations copied by default";box.appendChild(nn);return;}fs.forEach(function(f){nf++;var row=document.createElement("label");row.className="tfold";var cb=document.createElement("input");cb.type="checkbox";cb.value=f.path;cb.setAttribute("data-bytes",f.bytes||0);cb.setAttribute("data-exists",f.exists?"1":"0");cb.checked=!!f.exists;cb.onchange=recalcTotal;var nm=document.createElement("span");nm.className="nm";nm.textContent=f.path+(f.exists?"":" (missing)");var sz=document.createElement("span");sz.className="sz";sz.textContent=f.exists?fmtBytes(f.bytes):"";row.appendChild(cb);row.appendChild(nm);row.appendChild(sz);box.appendChild(row);if(f.exists&&!(f.path in uniq))uniq[f.path]=parseInt(f.bytes||0);});});});ALLSIZE=0;Object.keys(uniq).forEach(function(k){ALLSIZE+=uniq[k];});var opt=document.querySelector('#e_connmode option[value=all]');if(opt)opt.textContent="All of it ("+fmtBytes(ALLSIZE)+")";st.textContent="Done — "+np+" project(s), "+nf+" connected folder(s). Claude core ≈ "+fmtBytes(SOUL)+".";document.getElementById("e_gate").classList.remove("gated");connToggle();var wv=document.getElementById("e_warn");if(wv){wv.innerHTML="";(j.warnings||[]).forEach(function(wm){var dw=document.createElement("div");dw.className="warnline";dw.textContent="\u26a0 "+wm;wv.appendChild(dw);});}}catch(e){st.textContent="Scan failed: "+e;}sb.disabled=false;}
async function showDrives(){var r=await fetch("/api/drives");var j=await r.json();var list=document.getElementById("plist");list.innerHTML="";document.getElementById("pcur").textContent="Drives / locations";(j.drives||[]).forEach(function(d){var el=document.createElement("div");el.className="pitem";el.textContent="[drive] "+d;el.onclick=function(){openAt(d);};list.appendChild(el);});}
async function streamBar(url,body,barId,fillId){var L=document.getElementById("log");L.textContent="Working…";var bar=document.getElementById(barId),fill=document.getElementById(fillId);bar.style.display="block";fill.style.width="0%";var pctEl=document.getElementById(barId+"_pct"),stEl=document.getElementById(barId+"_stage");if(pctEl){pctEl.style.display="inline";pctEl.textContent="0%";}if(stEl){stEl.style.display="none";stEl.textContent="";}var r=await fetch(url,{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify(body)});var rd=r.body.getReader(),dec=new TextDecoder(),bufp="",log="";while(true){var x=await rd.read();if(x.done)break;bufp+=dec.decode(x.value,{stream:true});var lines=bufp.split("\n");bufp=lines.pop();lines.forEach(function(ln){var m=ln.match(/^@@P@@(\d+)$/);if(m){fill.style.width=m[1]+"%";if(pctEl)pctEl.textContent=m[1]+"%";}else{var st=ln.match(/^@@S@@(.*)$/);if(st){if(stEl){stEl.style.display="block";stEl.textContent=st[1];}}else{var pp=ln.match(/^@@POP@@(.*)$/);if(pp){try{showConnPopup(JSON.parse(pp[1]));}catch(e){}}else{log+=ln+"\n";}}}});L.textContent=log;L.scrollTop=L.scrollHeight;}if(bufp)L.textContent=log+bufp;fill.style.width="100%";if(pctEl)pctEl.textContent="100%";if(stEl)stEl.style.display="none";}
function doExport(){var on=document.getElementById("e_conn").checked;var mode=on?document.getElementById("e_connmode").value:"core";var folders=[];if(mode=="partial"){document.querySelectorAll(".tfold input:checked").forEach(function(c){folders.push(c.value);});}streamBar("/api/export",{cowork:gv("e_cowork"),claudeCode:gv("e_cc"),outDir:gv("e_out"),includeSkills:true,mode:mode,folders:folders},"e_bar","e_barfill");}
function doImport(dry){streamBar("/api/import",{pkgDir:gv("i_pkg"),merge:document.getElementById("i_merge").checked,dryRun:!!dry},"i_bar","i_barfill");}
</script></body></html>`
