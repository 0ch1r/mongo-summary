package report

import (
	"html/template"
	"os"

	"github.com/jerichorivera/mongo-summary/internal/collector"
)

func WriteHTML(path string, summary collector.Summary) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return page.Execute(f, summary)
}

var page = template.Must(template.New("page").Funcs(template.FuncMap{
	"dict": func(values ...any) map[string]any {
		out := make(map[string]any, len(values)/2)
		for i := 0; i+1 < len(values); i += 2 {
			key, _ := values[i].(string)
			out[key] = values[i+1]
		}
		return out
	},
}).Parse(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.Title}} MongoDB Summary</title>
<style>
:root {
  --bg: #f6f7f9; --panel: #fff; --text: #17202a; --muted: #667085; --line: #d7dce2; --accent: #1b6f5f;
}
@media (prefers-color-scheme: dark) {
  :root {
    --bg: #0f1419; --panel: #1a1f2e; --text: #e1e7ef; --muted: #8896a6; --line: #2d3548; --accent: #3dd68c;
  }
}
*{box-sizing:border-box}
body{margin:0;background:var(--bg);color:var(--text);font-family:Inter,ui-sans-serif,system-ui,-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif;font-size:14px;line-height:1.45}
header{background:#173b43;color:white;padding:24px 28px 20px}
header h1{margin:0 0 6px;font-size:26px}
header p{margin:0;color:#d6e4e6}
main{max-width:1280px;margin:0 auto;padding:20px}
.grid{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:12px}
.two{display:grid;grid-template-columns:1fr 1fr;gap:16px}
section{margin:0 0 16px}
.panel{background:var(--panel);border:1px solid var(--line);border-radius:8px;padding:14px}
h2{font-size:18px;margin:0;padding:2px 0 10px;cursor:pointer;user-select:none}
h3{font-size:15px;margin:0 0 10px}
.collapsed{display:none}
.metric{background:var(--panel);border:1px solid var(--line);border-radius:8px;padding:12px}
.metric b{display:block;font-size:20px}
.metric span{color:var(--muted);font-size:12px}
table{width:100%;border-collapse:collapse}
th,td{text-align:left;padding:8px 10px;border-bottom:1px solid var(--line);vertical-align:top}
th{font-size:12px;color:var(--muted);font-weight:700;background:#f9fafb;white-space:nowrap}
th.sortable{cursor:pointer;user-select:none;position:relative;padding-right:22px}
th.sort-asc::after,th.sort-desc::after{position:absolute;right:6px;font-size:10px;color:var(--accent)}
th.sort-asc::after{content:"\25B2"}
th.sort-desc::after{content:"\25BC"}
.badge{display:inline-block;border-radius:999px;padding:2px 8px;font-size:12px;font-weight:700;background:#eef2f3;color:#344054}
.PRIMARY{background:#dff3e8;color:#257044}.SECONDARY{background:#eaf0ff;color:#284a8a}.ARBITER{background:#fff2d8;color:#9a5b00}
.tree ul{list-style:none;margin:0 0 0 22px;padding:0;border-left:1px solid var(--line)}
.tree li{position:relative;margin:8px 0;padding-left:14px}
.tree li:before{content:"";position:absolute;left:0;top:14px;width:10px;border-top:1px solid var(--line)}
.node{display:inline-block;background:var(--panel);border:1px solid var(--line);border-radius:8px;padding:8px 10px;min-width:260px}
.node strong{display:block}
.muted{color:var(--muted)}
pre{white-space:pre-wrap;overflow:auto;background:#101828;color:#f2f4f7;padding:12px;border-radius:8px;max-height:320px}
#back-to-top{position:fixed;right:18px;bottom:18px;border:1px solid var(--line);background:var(--panel);color:var(--text);border-radius:999px;padding:10px 12px;cursor:pointer;box-shadow:0 2px 8px #00000022;display:none}
@media (max-width:900px){.grid,.two{grid-template-columns:1fr}}
</style>
</head>
<body>
<header>
<h1>{{.Title}} MongoDB Summary</h1>
<p>{{.InputDir}} · generated {{.GeneratedAt.Format "2006-01-02 15:04:05 MST"}}</p>
</header>
<main>
<section class="grid">
<div class="metric"><b>{{len .Files}}</b><span>files found</span></div>
<div class="metric"><b>{{.Log.SlowQueries.Count}}</b><span>slow query log lines</span></div>
<div class="metric"><b>{{.Log.Lines}}</b><span>mongod.log lines</span></div>
</section>

<section class="two">
<div class="panel"><h2>Static Log Overview</h2><table><tbody><tr><th>Lines</th><td>{{.Log.Lines}}</td></tr><tr><th>Range</th><td>{{.Log.Start}} to {{.Log.End}}</td></tr><tr><th>Duplicate key messages</th><td>{{.Log.DuplicateKeyCount}}</td></tr><tr><th>Slow query max / avg</th><td>{{.Log.SlowQueries.MaxDurationMS}} ms / {{printf "%.1f" .Log.SlowQueries.AvgDurationMS}} ms</td></tr></tbody></table><h3>Severity</h3>{{template "counts" .Log.SeverityCounts}}</div>
<div class="panel"><h2>Top Log Messages (Static)</h2>{{template "counts" .Log.MessageCounts}}</div>
</section>
<section class="two">
<div class="panel"><h2>Top Slow Query Namespaces (Static)</h2>{{template "counts" .Log.SlowQueries.NamespaceCount}}</div>
<div class="panel"><h2>Top Slow Query Shapes (Static)</h2>{{if .Log.SlowQueries.Shapes}}<table><thead><tr><th>Query hash</th><th>Plan cache key</th><th>Namespace</th><th>Command</th><th>Count</th><th>Max</th><th>Avg</th></tr></thead><tbody>{{range .Log.SlowQueries.Shapes}}<tr><td><code>{{.QueryHash}}</code></td><td><code>{{.PlanCacheKey}}</code></td><td><code>{{.Namespace}}</code></td><td>{{.Command}}</td><td>{{.Count}}</td><td>{{.MaxDurationMS}} ms</td><td>{{printf "%.1f" .AvgDurationMS}} ms</td></tr>{{end}}</tbody></table>{{else}}<p class="muted">No queryHash/planCacheKey values found in slow logs.</p>{{end}}</div>
</section>

{{range $i, $snap := .Snapshots}}
<section class="two">
<div class="panel">
<h2>Server</h2>
<table><tbody>
<tr><th>Host</th><td>{{$snap.Server.Host}}</td></tr><tr><th>Process</th><td>{{$snap.Server.Process}} pid {{$snap.Server.PID}}</td></tr>
<tr><th>Version / FCV</th><td>{{$snap.Server.Version}} / {{$snap.Server.FCV}}</td></tr><tr><th>Uptime</th><td>{{$snap.Server.Uptime}} seconds</td></tr>
<tr><th>Connections</th><td>{{$snap.Server.ConnectionsCurrent}} current, {{$snap.Server.ConnectionsActive}} active, {{$snap.Server.ConnectionsAvailable}} available</td></tr>
<tr><th>Default RW Concern</th><td>read={{$snap.Server.DefaultReadConcern}}, write={{$snap.Server.DefaultWriteConcern}}</td></tr>
<tr><th>Collections</th><td>{{$snap.Server.CatalogCollections}}</td></tr>
</tbody></table>
</div>
<div class="panel">
<h2>Replica Set Health</h2>
<table><tbody>
<tr><th>Primary</th><td>{{$snap.ReplicaSet.Primary}}</td></tr><tr><th>Term</th><td>{{$snap.ReplicaSet.Term}}</td></tr>
<tr><th>Majority</th><td>votes={{$snap.ReplicaSet.MajorityVoteCount}}, writes={{$snap.ReplicaSet.WriteMajorityCount}}</td></tr>
<tr><th>Voting members</th><td>{{$snap.ReplicaSet.VotingMembersCount}} total, {{$snap.ReplicaSet.WritableVotingMembers}} writable</td></tr>
<tr><th>Oplog window</th><td>{{$snap.ReplicaSet.Oplog.Window}}</td></tr><tr><th>Oplog size</th><td>{{$snap.ReplicaSet.Oplog.ConfiguredSize}}</td></tr>
</tbody></table>
</div>
</section>

<section class="panel">
<h2>RED — Request Rate, Error Rate, Duration</h2>
{{if $snap.RED.Available}}
<p class="muted">Per-command rates are lifetime averages over server uptime ({{printf "%.0f" $snap.RED.UptimeSec}} s).</p>
{{if $snap.RED.Commands}}<table><thead><tr><th>Command</th><th>Total</th><th>Failed</th><th>Req/s</th><th>Err/s</th><th>Err %</th><th>Slow count</th><th>Slow P95</th><th>Slow P99</th></tr></thead><tbody>{{range $snap.RED.Commands}}<tr><td><code>{{.Name}}</code></td><td>{{.Total}}</td><td>{{.Failed}}</td><td>{{printf "%.2f" .RequestsPerSec}}</td><td>{{printf "%.4f" .ErrorsPerSec}}</td><td>{{printf "%.2f" .ErrorPercent}}%</td><td>{{.SlowCount}}</td><td>{{.SlowP95MS}} ms</td><td>{{.SlowP99MS}} ms</td></tr>{{end}}</tbody></table>{{else}}<p class="muted">No per-command metrics found in serverStatus.metrics.commands.</p>{{end}}
{{else}}<p class="muted">No serverStatus document was available, so RED metrics could not be computed.</p>{{end}}
</section>

<section class="panel">
<h2>WiredTiger Cache and Eviction Health</h2>
{{if $snap.WiredTiger.Available}}
<div class="grid">
<div class="metric"><b>{{printf "%.1f%%" $snap.WiredTiger.CacheUsedPercent}}</b><span>cache used</span></div>
<div class="metric"><b>{{printf "%.2f%%" $snap.WiredTiger.DirtyCachePercent}}</b><span>dirty of configured cache</span></div>
<div class="metric"><b>{{$snap.WiredTiger.CacheTimeouts}}</b><span>cache-space timeouts</span></div>
<div class="metric"><b>{{$snap.WiredTiger.AggressiveMode}}</b><span>aggressive eviction flag</span></div>
</div>
{{else}}<p class="muted">No WiredTiger cache statistics were found in serverStatus.</p>{{end}}
</section>

<section class="panel">
<h2>Replica Set Topology</h2>
<div class="tree">{{range $j, $m := $snap.ReplicaSet.Members}}{{if eq $m.SyncSourceHost ""}}{{template "node" dict "Member" $m "Members" $snap.ReplicaSet.Members}}{{end}}{{end}}</div>
<h3>Members</h3>
<table><thead><tr><th>ID</th><th>Host</th><th>Site</th><th>State</th><th>Lag</th><th>Ping</th><th>Sync source</th><th>Priority</th><th>Votes</th></tr></thead><tbody>{{range $snap.ReplicaSet.Members}}<tr><td>{{.ID}}</td><td><code>{{.Name}}</code></td><td>{{.Site}}</td><td><span class="badge {{.State}}">{{.State}}</span></td><td>{{.ReplicationLag}}</td><td>{{.PingMS}} ms</td><td><code>{{.SyncSourceHost}}</code></td><td>{{.Priority}}</td><td>{{.Votes}}</td></tr>{{end}}</tbody></table>
</section>

<section class="two">
<div class="panel"><h2>Command-Line Options</h2><p class="muted">Source: {{$snap.CommandLine.Source}}</p>{{if $snap.CommandLine.Options}}<table><thead><tr><th>Option</th><th>Value</th></tr></thead><tbody>{{range $k,$v := $snap.CommandLine.Options}}<tr><td><code>{{$k}}</code></td><td><code>{{$v}}</code></td></tr>{{end}}</tbody></table>{{else}}<p class="muted">No command-line options were parsed.</p>{{end}}</div>
<div class="panel"><h2>Current Operations</h2><table><tbody><tr><th>Total</th><td>{{$snap.CurrentOps.Total}}</td></tr><tr><th>Active</th><td>{{$snap.CurrentOps.Active}}</td></tr><tr><th>Waiting for lock</th><td>{{$snap.CurrentOps.WaitingForLock}}</td></tr><tr><th>Waiting for flow control</th><td>{{$snap.CurrentOps.WaitingForFlow}}</td></tr></tbody></table><h3>Operation Mix</h3>{{template "counts" $snap.CurrentOps.Ops}}</div>
</section>

<section class="panel">
<h2>Parameter Highlights</h2>
{{if $snap.Parameters.Highlights}}<table><thead><tr><th>Parameter</th><th>Value</th></tr></thead><tbody>{{range $k,$v := $snap.Parameters.Highlights}}<tr><td><code>{{$k}}</code></td><td><code>{{$v}}</code></td></tr>{{end}}</tbody></table>{{else}}<p class="muted">No parameter highlights parsed.</p>{{end}}
</section>
{{end}}

<section class="panel">
<h2>Notes and Recommended Additions</h2>
<ul>{{range .Notes}}<li>{{.}}</li>{{end}}</ul>
</section>

<section class="panel">
<h2>Collection Inventory</h2>
<table><thead><tr><th>File</th><th>Lines</th><th>Size</th></tr></thead><tbody>{{range .Files}}<tr><td><code>{{.Name}}</code></td><td>{{.Lines}}</td><td>{{.Size}} bytes</td></tr>{{end}}</tbody></table>
</section>
{{range .RawArtifacts}}<section class="panel"><h2>{{.Title}}</h2><pre>{{.Content}}</pre></section>{{end}}
</main>
<button id="back-to-top" type="button" aria-label="Scroll back to top">↑ Top</button>
<script>
(function(){'use strict';
document.querySelectorAll('h2').forEach(function(h2){
  h2.addEventListener('click', function(){
    var sib = h2.nextElementSibling;
    var siblings = [];
    while(sib){ siblings.push(sib); sib = sib.nextElementSibling; }
    siblings.forEach(function(el){ el.classList.toggle('collapsed'); });
  });
});
function sortTable(tbody, col, dir){
  var rows = Array.from(tbody.querySelectorAll('tr'));
  rows.sort(function(a,b){
    var aTxt=(a.children[col]||{}).textContent||''; var bTxt=(b.children[col]||{}).textContent||'';
    aTxt=aTxt.trim(); bTxt=bTxt.trim();
    var aNum=parseFloat(aTxt.replace(/[^0-9.\-]/g,'')); var bNum=parseFloat(bTxt.replace(/[^0-9.\-]/g,''));
    if(!isNaN(aNum)&&!isNaN(bNum)){ return dir==='asc' ? aNum-bNum : bNum-aNum; }
    return dir==='asc' ? aTxt.localeCompare(bTxt) : bTxt.localeCompare(aTxt);
  });
  rows.forEach(function(r){ tbody.appendChild(r); });
}
document.querySelectorAll('table').forEach(function(tbl){
  var thead=tbl.querySelector('thead'); var tbody=tbl.querySelector('tbody');
  if(!thead||!tbody||tbody.querySelectorAll('tr').length<2) return;
  var ths=thead.querySelectorAll('th');
  ths.forEach(function(th, i){
    th.classList.add('sortable');
    th.addEventListener('click', function(){
      var dir='asc'; if(th.classList.contains('sort-asc')) dir='desc';
      ths.forEach(function(x){ x.classList.remove('sort-asc','sort-desc'); });
      th.classList.add(dir==='asc'?'sort-asc':'sort-desc');
      sortTable(tbody, i, dir);
    });
  });
});
var topBtn=document.getElementById('back-to-top');
window.addEventListener('scroll', function(){ topBtn.style.display = window.scrollY > 400 ? 'block' : 'none'; });
topBtn.addEventListener('click', function(){ window.scrollTo({top:0, behavior:'smooth'}); });
})();
</script>
</body>
</html>
{{define "counts"}}<table><thead><tr><th>Value</th><th>Count</th></tr></thead><tbody>{{range .}}<tr><td><code>{{.Key}}</code></td><td>{{.Count}}</td></tr>{{end}}</tbody></table>{{end}}
{{define "node"}}{{$m := .Member}}{{$members := .Members}}<div class="node"><strong>{{$m.HostShort}} <span class="badge {{$m.State}}">{{$m.State}}</span></strong><span class="muted">lag {{$m.ReplicationLag}} · ping {{$m.PingMS}} ms · site {{$m.Site}}</span></div>{{if $m.Children}}<ul>{{range $idx := $m.Children}}<li>{{template "node" dict "Member" (index $members $idx) "Members" $members}}</li>{{end}}</ul>{{end}}{{end}}
`))