package collector

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

func parseServerSummary(doc map[string]any) ServerSummary {
	fcv := dotted(doc, "featureCompatibilityVersion", "version")
	if fcv == "" {
		major := dotted(doc, "featureCompatibilityVersion", "major")
		minor := dotted(doc, "featureCompatibilityVersion", "minor")
		if major != "" && minor != "" {
			fcv = major + "." + minor
		}
	}
	return ServerSummary{
		Host:                 dotted(doc, "host"),
		Version:              dotted(doc, "version"),
		Process:              dotted(doc, "process"),
		PID:                  dotted(doc, "pid"),
		Uptime:               dotted(doc, "uptime"),
		LocalTime:            dotted(doc, "localTime"),
		FCV:                  fcv,
		ConnectionsCurrent:   dotted(doc, "connections", "current"),
		ConnectionsActive:    dotted(doc, "connections", "active"),
		ConnectionsAvailable: dotted(doc, "connections", "available"),
		DefaultReadConcern:   dotted(doc, "defaultRWConcern", "defaultReadConcern", "level"),
		DefaultWriteConcern:  dotted(doc, "defaultRWConcern", "defaultWriteConcern", "w"),
		UserAssertions:       dotted(doc, "asserts", "user"),
		CatalogCollections:   dotted(doc, "catalogStats", "collections"),
	}
}

func parseWiredTigerSummary(doc map[string]any) WiredTigerSummary {
	cache := getMap(getMap(doc, "wiredTiger"), "cache")
	capacity := getMap(getMap(doc, "wiredTiger"), "capacity")
	if len(cache) == 0 {
		return WiredTigerSummary{}
	}

	cacheBytes := intValue(cache["bytes currently in the cache"])
	maxCacheBytes := intValue(cache["maximum bytes configured"])
	dirtyBytes := intValue(cache["tracked dirty bytes in the cache"])
	forcedSelected := intValue(cache["forced eviction - pages selected count"])
	forcedFailed := intValue(cache["forced eviction - pages selected unable to be evicted count"])
	checkpointBlocked := intValue(cache["checkpoint blocked page eviction"]) + intValue(cache["checkpoint of history store file blocked non-history store page eviction"])
	hazardBlocked := intValue(cache["hazard pointer blocked page eviction"])
	evictionNoProgress := intValue(cache["eviction server slept, because we did not make progress with eviction"])
	unableGoal := intValue(cache["eviction server unable to reach eviction goal"])
	aggressiveMode := intValue(cache["eviction currently operating in aggressive mode"])
	cacheWaitOps := intValue(cache["application thread operations waiting for cache"])
	cacheWaitTime := intValue(cache["application thread time waiting for cache (usecs)"])
	cacheTimeouts := intValue(cache["operations timed out waiting for space in cache"])
	capacityWaitEviction := intValue(capacity["time waiting during eviction (usecs)"])

	wt := WiredTigerSummary{
		Available:              true,
		CacheBytes:             formatBytes(cacheBytes),
		MaxCacheBytes:          formatBytes(maxCacheBytes),
		CacheUsedPercent:       percent(cacheBytes, maxCacheBytes),
		TrackedDirtyBytes:      formatBytes(dirtyBytes),
		DirtyCachePercent:      percent(dirtyBytes, maxCacheBytes),
		DirtyUsedPercent:       percent(dirtyBytes, cacheBytes),
		PagesHeld:              formatInt(intValue(cache["pages currently held in the cache"])),
		PagesRead:              formatInt(intValue(cache["pages read into cache"])),
		PagesWritten:           formatInt(intValue(cache["pages written from cache"])),
		PagesRequested:         formatInt(intValue(cache["pages requested from the cache"])),
		ModifiedPagesEvicted:   formatInt(intValue(cache["modified pages evicted"])),
		UnmodifiedPagesEvicted: formatInt(intValue(cache["unmodified pages evicted"])),
		AppThreadEvictedPages:  formatInt(intValue(cache["pages evicted by application threads"])),
		ForcedEvictionSelected: formatInt(forcedSelected),
		ForcedEvictionFailed:   formatInt(forcedFailed),
		CheckpointBlocked:      formatInt(checkpointBlocked),
		HazardPointerBlocked:   formatInt(hazardBlocked),
		EvictionWalksAbandoned: formatInt(intValue(cache["eviction walks abandoned"])),
		EvictionNoProgress:     formatInt(evictionNoProgress),
		UnableToReachGoal:      formatInt(unableGoal),
		AggressiveMode:         formatInt(aggressiveMode),
		CacheWaitOps:           formatInt(cacheWaitOps),
		CacheWaitTime:          formatDurationMicros(cacheWaitTime),
		CacheTimeouts:          formatInt(cacheTimeouts),
		CapacityWaitEviction:   formatDurationMicros(capacityWaitEviction),
	}
	wt.Observations = wiredTigerObservations(wt, forcedSelected, forcedFailed, checkpointBlocked, hazardBlocked, evictionNoProgress, unableGoal, aggressiveMode, cacheWaitOps, cacheWaitTime, cacheTimeouts, capacityWaitEviction)
	return wt
}

func wiredTigerObservations(wt WiredTigerSummary, forcedSelected, forcedFailed, checkpointBlocked, hazardBlocked, evictionNoProgress, unableGoal, aggressiveMode, cacheWaitOps, cacheWaitTime, cacheTimeouts, capacityWaitEviction int64) []HealthObservation {
	var obs []HealthObservation
	switch {
	case wt.CacheUsedPercent >= 95:
		obs = append(obs, HealthObservation{"bad", fmt.Sprintf("Cache is %.1f%% full; eviction pressure is likely high.", wt.CacheUsedPercent)})
	case wt.CacheUsedPercent >= 79.5:
		obs = append(obs, HealthObservation{"warn", fmt.Sprintf("Cache is %.1f%% full; monitor for sustained pressure.", wt.CacheUsedPercent)})
	default:
		obs = append(obs, HealthObservation{"ok", fmt.Sprintf("Cache usage is %.1f%% of configured capacity.", wt.CacheUsedPercent)})
	}
	switch {
	case wt.DirtyCachePercent >= 20:
		obs = append(obs, HealthObservation{"bad", fmt.Sprintf("Tracked dirty cache is %.2f%% of configured cache.", wt.DirtyCachePercent)})
	case wt.DirtyCachePercent >= 5:
		obs = append(obs, HealthObservation{"warn", fmt.Sprintf("Tracked dirty cache is %.2f%% of configured cache.", wt.DirtyCachePercent)})
	default:
		obs = append(obs, HealthObservation{"ok", fmt.Sprintf("Tracked dirty cache is low at %.2f%% of configured cache.", wt.DirtyCachePercent)})
	}
	if aggressiveMode > 0 || unableGoal > 0 || cacheTimeouts > 0 {
		obs = append(obs, HealthObservation{"bad", "Eviction reported aggressive mode, goal misses, or cache-space timeouts."})
	}
	if cacheWaitOps > 0 || cacheWaitTime > 0 || capacityWaitEviction > 0 {
		obs = append(obs, HealthObservation{"warn", "Application or capacity throttling waits were recorded for cache/eviction."})
	}
	if forcedSelected > 0 {
		level := "warn"
		if percent(forcedFailed, forcedSelected) > 25 {
			level = "bad"
		}
		obs = append(obs, HealthObservation{level, fmt.Sprintf("Forced eviction selected %s pages; %s could not be evicted.", formatInt(forcedSelected), formatInt(forcedFailed))})
	}
	if checkpointBlocked > 0 {
		obs = append(obs, HealthObservation{"warn", fmt.Sprintf("Checkpoint activity blocked %s page evictions.", formatInt(checkpointBlocked))})
	}
	if hazardBlocked > 0 {
		obs = append(obs, HealthObservation{"warn", fmt.Sprintf("Hazard pointers blocked %s page evictions.", formatInt(hazardBlocked))})
	}
	if evictionNoProgress > 0 {
		obs = append(obs, HealthObservation{"warn", fmt.Sprintf("Eviction server slept without progress %s times.", formatInt(evictionNoProgress))})
	}
	return obs
}

func parseReplicaStatus(doc map[string]any) ReplicaSetSummary {
	rs := ReplicaSetSummary{
		Name:                  dotted(doc, "set"),
		Date:                  dotted(doc, "date"),
		Term:                  dotted(doc, "term"),
		MyState:               dotted(doc, "myState"),
		MajorityVoteCount:     dotted(doc, "majorityVoteCount"),
		WriteMajorityCount:    dotted(doc, "writeMajorityCount"),
		VotingMembersCount:    dotted(doc, "votingMembersCount"),
		WritableVotingMembers: dotted(doc, "writableVotingMembersCount"),
		ConfigSettings:        map[string]string{},
	}
	for _, raw := range getSlice(doc, "members") {
		m, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		member := ReplicaMember{
			ID:             str(m["_id"]),
			Name:           str(m["name"]),
			HostShort:      shortHost(str(m["name"])),
			Site:           siteFromHost(str(m["name"])),
			State:          str(m["stateStr"]),
			Health:         str(m["health"]),
			Uptime:         str(m["uptime"]),
			PingMS:         str(m["pingMs"]),
			SyncSourceHost: str(m["syncSourceHost"]),
			OptimeDate:     str(m["optimeDate"]),
			Self:           str(m["self"]) == "true",
		}
		if member.State == "PRIMARY" {
			rs.Primary = member.Name
		}
		rs.Members = append(rs.Members, member)
	}
	buildReplicaChildren(&rs)
	return rs
}

func mergeReplicaConfig(rs *ReplicaSetSummary, doc map[string]any) {
	if rs.Name == "" {
		rs.Name = dotted(doc, "_id")
	}
	configByHost := map[string]map[string]any{}
	for _, raw := range getSlice(doc, "members") {
		m, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		configByHost[str(m["host"])] = m
	}
	for i := range rs.Members {
		if cfg := configByHost[rs.Members[i].Name]; cfg != nil {
			rs.Members[i].Priority = str(cfg["priority"])
			rs.Members[i].Votes = str(cfg["votes"])
			rs.Members[i].ArbiterOnly = str(cfg["arbiterOnly"])
			rs.Members[i].Hidden = str(cfg["hidden"])
		}
	}
	settings := getMap(doc, "settings")
	keys := []string{"chainingAllowed", "heartbeatIntervalMillis", "heartbeatTimeoutSecs", "electionTimeoutMillis", "catchUpTimeoutMillis", "catchUpTakeoverDelayMillis"}
	if rs.ConfigSettings == nil {
		rs.ConfigSettings = map[string]string{}
	}
	for _, key := range keys {
		rs.ConfigSettings[key] = str(settings[key])
	}
	buildReplicaChildren(rs)
}

func buildReplicaChildren(rs *ReplicaSetSummary) {
	indexByName := map[string]int{}
	for i, m := range rs.Members {
		indexByName[m.Name] = i
		rs.Members[i].Children = nil
	}
	for i, m := range rs.Members {
		if m.SyncSourceHost == "" {
			continue
		}
		if parent, ok := indexByName[m.SyncSourceHost]; ok {
			rs.Members[parent].Children = append(rs.Members[parent].Children, i)
		}
	}
}

func mergeReplicaLag(rs *ReplicaSetSummary, path string) {
	sourceRE := regexp.MustCompile(`^source:\s+(.+)$`)
	lagRE := regexp.MustCompile(`^\s*([0-9]+)\s+secs.*behind the primary`)
	current := ""
	lags := map[string]string{}
	_ = readLines(path, func(line string) {
		if m := sourceRE.FindStringSubmatch(line); m != nil {
			current = strings.TrimSpace(m[1])
			return
		}
		if m := lagRE.FindStringSubmatch(line); m != nil && current != "" {
			lags[current] = m[1] + "s"
		}
	})
	for i := range rs.Members {
		if lag := lags[rs.Members[i].Name]; lag != "" {
			rs.Members[i].ReplicationLag = lag
		} else if rs.Members[i].State == "PRIMARY" {
			rs.Members[i].ReplicationLag = "0s"
		}
	}
}

func parseOplogSummary(path string) OplogSummary {
	var o OplogSummary
	_ = readLines(path, func(line string) {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "configured oplog size:"):
			o.ConfiguredSize = strings.TrimSpace(strings.TrimPrefix(line, "configured oplog size:"))
		case strings.HasPrefix(line, "log length start to end:"):
			o.Window = strings.TrimSpace(strings.TrimPrefix(line, "log length start to end:"))
		case strings.HasPrefix(line, "oplog first event time:"):
			o.FirstEvent = strings.TrimSpace(strings.TrimPrefix(line, "oplog first event time:"))
		case strings.HasPrefix(line, "oplog last event time:"):
			o.LastEvent = strings.TrimSpace(strings.TrimPrefix(line, "oplog last event time:"))
		case strings.HasPrefix(line, "now:"):
			o.Now = strings.TrimSpace(strings.TrimPrefix(line, "now:"))
		}
	})
	return o
}

func parseCommandLineFile(path string) CommandLineSummary {
	b, err := os.ReadFile(path)
	out := CommandLineSummary{Source: filepath.Base(path), Options: map[string]string{}}
	if err != nil {
		out.Errors = append(out.Errors, err.Error())
		return out
	}
	text := string(b)
	if strings.Contains(text, "Authentication failed") || strings.Contains(text, "exception: connect failed") {
		out.Errors = append(out.Errors, strings.TrimSpace(text))
		return out
	}
	if doc, err := parseMongoShellFile(path); err == nil {
		flatten("", getMap(doc, "parsed"), out.Options)
		flatten("", getMap(doc, "argv"), out.Options)
	}
	return out
}

func parseCurrentOps(doc map[string]any) CurrentOpSummary {
	var out CurrentOpSummary
	opCounts := map[string]int{}
	nsCounts := map[string]int{}
	driverCounts := map[string]int{}
	for _, raw := range getSlice(doc, "inprog") {
		op, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		out.Total++
		if str(op["active"]) == "true" {
			out.Active++
		}
		if str(op["waitingForLock"]) == "true" {
			out.WaitingForLock++
		}
		if str(op["waitingForFlowControl"]) == "true" {
			out.WaitingForFlow++
		}
		opCounts[str(op["op"])]++
		nsCounts[str(op["ns"])]++
		driver := dotted(op, "clientMetadata", "driver", "name") + " " + dotted(op, "clientMetadata", "driver", "version")
		driverCounts[strings.TrimSpace(driver)]++
		out.Longest = append(out.Longest, OperationSample{
			Op:          str(op["op"]),
			Namespace:   str(op["ns"]),
			Client:      str(op["client"]),
			SecsRunning: str(op["secs_running"]),
			Description: str(op["desc"]),
		})
	}
	sort.Slice(out.Longest, func(i, j int) bool {
		return number(out.Longest[i].SecsRunning) > number(out.Longest[j].SecsRunning)
	})
	if len(out.Longest) > 10 {
		out.Longest = out.Longest[:10]
	}
	out.Ops = sortedCounts(opCounts, 12)
	out.Namespaces = sortedCounts(nsCounts, 12)
	out.Drivers = sortedCounts(driverCounts, 8)
	return out
}

func parseParameters(doc map[string]any) ParameterSummary {
	interesting := []string{
		"featureCompatibilityVersion", "authenticationMechanisms", "clusterAuthMode",
		"enableFlowControl", "changeSyncSourceThresholdMillis", "diagnosticDataCollectionEnabled",
		"diagnosticDataCollectionPeriodMillis", "connPoolMaxConnsPerHost", "cursorTimeoutMillis",
		"allowDiskUseByDefault", "enableDetailedConnectionHealthMetricLogLines",
	}
	out := ParameterSummary{Highlights: map[string]string{}}
	for _, key := range interesting {
		if v, ok := doc[key]; ok {
			out.Highlights[key] = str(v)
		}
	}
	return out
}

type logHeaderRaw struct {
	Timestamp []byte
	Severity  []byte
	Component []byte
	Message   []byte
	Attr      []byte
}

var (
	logKeyT    = []byte("t")
	logKeyS    = []byte("s")
	logKeyC    = []byte("c")
	logKeyMsg  = []byte("msg")
	logKeyAttr = []byte("attr")
	logKeyDate = []byte("$date")
)

func scanLogLine(line []byte) (logHeaderRaw, bool) {
	var out logHeaderRaw
	i := skipJSONSpace(line, 0)
	if i >= len(line) || line[i] != '{' {
		return out, false
	}
	i++
	for {
		i = skipJSONSpace(line, i)
		if i >= len(line) {
			return out, false
		}
		if line[i] == '}' {
			return out, true
		}
		if line[i] != '"' {
			return out, false
		}
		ks, ke, next, ok := scanJSONString(line, i)
		if !ok {
			return out, false
		}
		key := line[ks:ke]
		i = skipJSONSpace(line, next)
		if i >= len(line) || line[i] != ':' {
			return out, false
		}
		i = skipJSONSpace(line, i+1)
		valStart := i

		switch {
		case bytes.Equal(key, logKeyT):
			if i < len(line) && line[i] == '{' {
				out.Timestamp = extractDateField(line, i)
			}
			i = skipJSONValue(line, valStart)
		case bytes.Equal(key, logKeyS):
			if i < len(line) && line[i] == '"' {
				vs, ve, n2, sok := scanJSONString(line, i)
				if sok {
					out.Severity = line[vs:ve]
					i = n2
					break
				}
			}
			i = skipJSONValue(line, valStart)
		case bytes.Equal(key, logKeyC):
			if i < len(line) && line[i] == '"' {
				vs, ve, n2, sok := scanJSONString(line, i)
				if sok {
					out.Component = line[vs:ve]
					i = n2
					break
				}
			}
			i = skipJSONValue(line, valStart)
		case bytes.Equal(key, logKeyMsg):
			if i < len(line) && line[i] == '"' {
				vs, ve, n2, sok := scanJSONString(line, i)
				if sok {
					out.Message = line[vs:ve]
					i = n2
					break
				}
			}
			i = skipJSONValue(line, valStart)
		case bytes.Equal(key, logKeyAttr):
			end := skipJSONValue(line, valStart)
			out.Attr = line[valStart:end]
			i = end
		default:
			i = skipJSONValue(line, valStart)
		}

		i = skipJSONSpace(line, i)
		if i >= len(line) {
			return out, false
		}
		if line[i] == ',' {
			i++
			continue
		}
		if line[i] == '}' {
			return out, true
		}
		return out, false
	}
}

func extractDateField(line []byte, i int) []byte {
	if i >= len(line) || line[i] != '{' {
		return nil
	}
	j := i + 1
	for {
		j = skipJSONSpace(line, j)
		if j >= len(line) || line[j] == '}' {
			return nil
		}
		if line[j] != '"' {
			return nil
		}
		ks, ke, next, ok := scanJSONString(line, j)
		if !ok {
			return nil
		}
		j = skipJSONSpace(line, next)
		if j >= len(line) || line[j] != ':' {
			return nil
		}
		j = skipJSONSpace(line, j+1)
		if bytes.Equal(line[ks:ke], logKeyDate) {
			if j < len(line) && line[j] == '"' {
				vs, ve, _, sok := scanJSONString(line, j)
				if sok {
					return line[vs:ve]
				}
			}
			return nil
		}
		j = skipJSONValue(line, j)
		j = skipJSONSpace(line, j)
		if j < len(line) && line[j] == ',' {
			j++
			continue
		}
		return nil
	}
}

func skipJSONSpace(b []byte, i int) int {
	for i < len(b) {
		switch b[i] {
		case ' ', '\t', '\n', '\r':
			i++
		default:
			return i
		}
	}
	return i
}

func scanJSONString(b []byte, i int) (innerStart, innerEnd, next int, ok bool) {
	if i >= len(b) || b[i] != '"' {
		return 0, 0, i, false
	}
	j := i + 1
	innerStart = j
	for j < len(b) {
		c := b[j]
		if c == '\\' {
			if j+1 >= len(b) {
				return 0, 0, j, false
			}
			j += 2
			continue
		}
		if c == '"' {
			return innerStart, j, j + 1, true
		}
		j++
	}
	return 0, 0, j, false
}

func skipJSONValue(b []byte, i int) int {
	i = skipJSONSpace(b, i)
	if i >= len(b) {
		return i
	}
	switch b[i] {
	case '"':
		_, _, next, ok := scanJSONString(b, i)
		if !ok {
			return len(b)
		}
		return next
	case '{':
		return skipJSONBracketed(b, i, '{', '}')
	case '[':
		return skipJSONBracketed(b, i, '[', ']')
	case 't', 'f', 'n':
		for i < len(b) {
			c := b[i]
			if c >= 'a' && c <= 'z' {
				i++
				continue
			}
			return i
		}
		return i
	default:
		for i < len(b) {
			c := b[i]
			if c == '-' || c == '+' || c == '.' || c == 'e' || c == 'E' || (c >= '0' && c <= '9') {
				i++
				continue
			}
			return i
		}
		return i
	}
}

func skipJSONBracketed(b []byte, i int, open, closeB byte) int {
	if i >= len(b) || b[i] != open {
		return i
	}
	j := i + 1
	for j < len(b) {
		switch b[j] {
		case '"':
			_, _, next, ok := scanJSONString(b, j)
			if !ok {
				return len(b)
			}
			j = next
			continue
		case '{':
			j = skipJSONBracketed(b, j, '{', '}')
			continue
		case '[':
			j = skipJSONBracketed(b, j, '[', ']')
			continue
		case closeB:
			return j + 1
		}
		j++
	}
	return j
}

type slowQueryAttr struct {
	NS             string         `json:"ns"`
	Command        map[string]any `json:"command"`
	WriteConcern   map[string]any `json:"writeConcern"`
	DurationMillis int64          `json:"durationMillis"`
	QueryHash      string         `json:"queryHash"`
	PlanCacheKey   string         `json:"planCacheKey"`
}

type optionsAttr struct {
	Options map[string]any `json:"options"`
}

var (
	dupKeyCamel = []byte("DuplicateKey")
	dupKeyLower = []byte("duplicate key")
)

func parseLogFile(path string) (LogSummary, map[string]string, map[string][]int, error) {
	var out LogSummary
	options := map[string]string{}
	severityCounts := map[string]int{}
	componentCounts := map[string]int{}
	messageCounts := map[string]int{}
	slowNS := map[string]int{}
	slowCommand := map[string]int{}
	slowShapes := map[string]*slowShapeAccumulator{}
	writeConcernDurations := map[string][]int{}
	slowDurationsByCommand := map[string][]int{}
	var slowTotal int
	slowQCtx := &slowQueryCtx{
		ns: slowNS, cmd: slowCommand,
		shapes: slowShapes, wcDurations: writeConcernDurations,
		cmdDurations: slowDurationsByCommand,
	}

	f, err := os.Open(path)
	if err != nil {
		return out, options, slowDurationsByCommand, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	buf := make([]byte, 1024*1024)
	scanner.Buffer(buf, 8*1024*1024)

	for scanner.Scan() {
		rawLine := scanner.Bytes()
		if len(rawLine) == 0 {
			continue
		}
		out.Lines++
		hdr, ok := scanLogLine(rawLine)
		if !ok {
			continue
		}
		ts := string(hdr.Timestamp)
		if out.Start == "" {
			out.Start = ts
		}
		out.End = ts
		sev := string(hdr.Severity)
		comp := string(hdr.Component)
		msg := string(hdr.Message)
		severityCounts[sev]++
		componentCounts[comp]++
		messageCounts[msg]++

		switch msg {
		case "Slow query":
			if len(hdr.Attr) == 0 {
				break
			}
			var sa slowQueryAttr
			if err := json.Unmarshal(hdr.Attr, &sa); err != nil {
				break
			}
				slowQCtx.process(hdr, sa, &out, &slowTotal, ts, sev, comp, msg)
		case "Options set by command line":
			if len(hdr.Attr) == 0 {
				break
			}
			var oa optionsAttr
			if err := json.Unmarshal(hdr.Attr, &oa); err == nil {
				flatten("", oa.Options, options)
			}
		}

		collectLogEvent(hdr, sev, comp, msg, ts, &out)

		if bytes.Contains(rawLine, dupKeyCamel) || bytes.Contains(rawLine, dupKeyLower) {
			out.DuplicateKeyCount++
		}
	}
	if err := scanner.Err(); err != nil {
		return out, options, slowDurationsByCommand, err
	}
	if out.SlowQueries.Count > 0 {
		out.SlowQueries.AvgDurationMS = float64(slowTotal) / float64(out.SlowQueries.Count)
	}
	out.SeverityCounts = sortedCounts(severityCounts, 8)
	out.ComponentCounts = sortedCounts(componentCounts, 12)
	out.MessageCounts = sortedCounts(messageCounts, 12)
	out.SlowQueries.NamespaceCount = sortedCounts(slowNS, 10)
	out.SlowQueries.CommandCount = sortedCounts(slowCommand, 10)
	out.SlowQueries.Shapes = sortedSlowQueryShapes(slowShapes, 15)
	out.SlowQueries.WriteConcernLatency = computeWriteConcernLatency(writeConcernDurations)
	return out, options, slowDurationsByCommand, nil
}

func parseRED(serverStatus map[string]any, log LogSummary, slowByCommand map[string][]int) REDSummary {
	if len(serverStatus) == 0 {
		return REDSummary{}
	}
	uptime := number(dotted(serverStatus, "uptime"))
	if uptime <= 0 {
		return REDSummary{}
	}
	out := REDSummary{Available: true, UptimeSec: uptime}

	opLatencies := getMap(serverStatus, "opLatencies")
	for _, cat := range []string{"reads", "writes", "commands", "transactions"} {
		m := getMap(opLatencies, cat)
		if len(m) == 0 {
			continue
		}
		ops := intValue(m["ops"])
		latency := intValue(m["latency"])
		rps := float64(ops) / uptime
		mean := 0.0
		if ops > 0 {
			mean = float64(latency) / float64(ops) / 1000.0
		}
		out.LatencyCategories = append(out.LatencyCategories, REDLatencyCategory{
			Category:       cat,
			Ops:            ops,
			RequestsPerSec: rps,
			MeanLatencyMS:  mean,
		})
	}

	metrics := getMap(getMap(serverStatus, "metrics"), "commands")
	var commands []REDCommand
	for name, raw := range metrics {
		m, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		total := intValue(m["total"])
		if total <= 0 {
			continue
		}
		failed := intValue(m["failed"])
		cmd := REDCommand{
			Name:           name,
			Total:          total,
			Failed:         failed,
			RequestsPerSec: float64(total) / uptime,
			ErrorsPerSec:   float64(failed) / uptime,
			ErrorPercent:   float64(failed) * 100 / float64(total),
		}
		if durations, ok := slowByCommand[name]; ok && len(durations) > 0 {
			sorted := append([]int(nil), durations...)
			sort.Ints(sorted)
			cmd.SlowCount = len(sorted)
			cmd.SlowP95MS = percentileInt(sorted, 95)
			cmd.SlowP99MS = percentileInt(sorted, 99)
		}
		commands = append(commands, cmd)
	}
	sort.Slice(commands, func(i, j int) bool {
		if commands[i].RequestsPerSec == commands[j].RequestsPerSec {
			return commands[i].Name < commands[j].Name
		}
		return commands[i].RequestsPerSec > commands[j].RequestsPerSec
	})
	if len(commands) > 20 {
		commands = commands[:20]
	}
	out.Commands = commands

	asserts := getMap(serverStatus, "asserts")
	out.Process.UserAsserts = intValue(asserts["user"])
	out.Process.UserAssertsPerSec = float64(out.Process.UserAsserts) / uptime

	logWindow := computeLogWindowSeconds(log.Start, log.End)
	out.LogWindowSec = logWindow

	var logErr int
	for _, c := range log.SeverityCounts {
		if c.Key == "E" || c.Key == "F" {
			logErr += c.Count
		}
	}
	out.Process.LogErrors = logErr
	out.Process.DuplicateKey = log.DuplicateKeyCount
	if logWindow > 0 {
		out.Process.LogErrorsPerSec = float64(logErr) / logWindow
		out.Process.DuplicateKeyPerSec = float64(log.DuplicateKeyCount) / logWindow
	}

	out.Observations = redObservations(out)
	return out
}

func redObservations(r REDSummary) []HealthObservation {
	var obs []HealthObservation
	for _, c := range r.Commands {
		switch {
		case c.ErrorPercent >= 5:
			obs = append(obs, HealthObservation{"bad", fmt.Sprintf("%s error rate is %.2f%% (%d failed of %d).", c.Name, c.ErrorPercent, c.Failed, c.Total)})
		case c.ErrorPercent >= 1:
			obs = append(obs, HealthObservation{"warn", fmt.Sprintf("%s error rate is %.2f%% (%d failed of %d).", c.Name, c.ErrorPercent, c.Failed, c.Total)})
		}
	}
	for _, l := range r.LatencyCategories {
		if l.Ops > 0 && l.MeanLatencyMS >= 100 {
			obs = append(obs, HealthObservation{"warn", fmt.Sprintf("Mean %s latency is %.2f ms over %d ops.", l.Category, l.MeanLatencyMS, l.Ops)})
		}
	}
	if r.Process.LogErrorsPerSec >= 1 {
		obs = append(obs, HealthObservation{"warn", fmt.Sprintf("Log severities E/F averaged %.2f/s over the log window.", r.Process.LogErrorsPerSec)})
	}
	return obs
}

func computeLogWindowSeconds(start, end string) float64 {
	if start == "" || end == "" {
		return 0
	}
	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04:05.000Z07:00",
		"2006-01-02T15:04:05.000-0700",
	}
	parse := func(s string) (time.Time, bool) {
		for _, l := range layouts {
			if t, err := time.Parse(l, s); err == nil {
				return t, true
			}
		}
		return time.Time{}, false
	}
	s, ok1 := parse(start)
	e, ok2 := parse(end)
	if !ok1 || !ok2 {
		return 0
	}
	diff := e.Sub(s).Seconds()
	if diff <= 0 {
		return 0
	}
	return diff
}

func isWriteCommand(name string) bool {
	switch name {
	case "insert", "update", "delete", "findAndModify", "bulkWrite", "commitTransaction":
		return true
	}
	return false
}

func formatWriteConcern(wc map[string]any) string {
	if len(wc) == 0 {
		return ""
	}
	var parts []string
	if v, ok := wc["w"]; ok {
		parts = append(parts, "w="+str(v))
	}
	if v, ok := wc["j"]; ok {
		parts = append(parts, "j="+str(v))
	}
	if v, ok := wc["wtimeout"]; ok && str(v) != "" && str(v) != "0" {
		parts = append(parts, "wtimeout="+str(v))
	}
	return strings.Join(parts, ", ")
}

func computeWriteConcernLatency(durationsByConcern map[string][]int) []WriteConcernLatency {
	out := make([]WriteConcernLatency, 0, len(durationsByConcern))
	for concern, durations := range durationsByConcern {
		if len(durations) == 0 {
			continue
		}
		sort.Ints(durations)
		sum := 0
		for _, d := range durations {
			sum += d
		}
		n := len(durations)
		out = append(out, WriteConcernLatency{
			Concern:       concern,
			Count:         n,
			MinDurationMS: durations[0],
			MaxDurationMS: durations[n-1],
			AvgDurationMS: float64(sum) / float64(n),
			P50DurationMS: percentileInt(durations, 50),
			P90DurationMS: percentileInt(durations, 90),
			P95DurationMS: percentileInt(durations, 95),
			P99DurationMS: percentileInt(durations, 99),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count == out[j].Count {
			return out[i].Concern < out[j].Concern
		}
		return out[i].Count > out[j].Count
	})
	return out
}

func percentileInt(sorted []int, p int) int {
	if len(sorted) == 0 {
		return 0
	}
	if p <= 0 {
		return sorted[0]
	}
	if p >= 100 {
		return sorted[len(sorted)-1]
	}
	idx := (p * (len(sorted) - 1)) / 100
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

type slowShapeAccumulator struct {
	QueryHash       string
	PlanCacheKey    string
	Namespace       string
	Command         string
	Count           int
	MaxDurationMS   int
	TotalDurationMS int
}

func sortedSlowQueryShapes(shapes map[string]*slowShapeAccumulator, limit int) []SlowQueryShape {
	out := make([]SlowQueryShape, 0, len(shapes))
	for _, shape := range shapes {
		avg := 0.0
		if shape.Count > 0 {
			avg = float64(shape.TotalDurationMS) / float64(shape.Count)
		}
		out = append(out, SlowQueryShape{
			QueryHash:     shape.QueryHash,
			PlanCacheKey:  shape.PlanCacheKey,
			Namespace:     shape.Namespace,
			Command:       shape.Command,
			Count:         shape.Count,
			MaxDurationMS: shape.MaxDurationMS,
			AvgDurationMS: avg,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count == out[j].Count {
			if out[i].MaxDurationMS == out[j].MaxDurationMS {
				return out[i].Namespace < out[j].Namespace
			}
			return out[i].MaxDurationMS > out[j].MaxDurationMS
		}
		return out[i].Count > out[j].Count
	})
	if limit > 0 && len(out) > limit {
		return out[:limit]
	}
	return out
}

// --------------------------------------------------------------------------
// Helpers extracted from parseLogFile
// --------------------------------------------------------------------------

// slowQueryCtx groups the accumulators updated when processing a Slow query log line.
type slowQueryCtx struct {
	ns           map[string]int
	cmd          map[string]int
	shapes       map[string]*slowShapeAccumulator
	wcDurations  map[string][]int
	cmdDurations map[string][]int
}

// process handles one "Slow query" log line, updating accumulators and the summary.
func (sc *slowQueryCtx) process(hdr logHeaderRaw, sa slowQueryAttr, out *LogSummary, slowTotal *int, ts, sev, comp, msg string) {
	out.SlowQueries.Count++
	ns := sa.NS
	sc.ns[ns]++
	cmdName := firstCommandName(sa.Command)
	sc.cmd[cmdName]++
	duration := int(sa.DurationMillis)
	*slowTotal += duration

	if cmdName != "" && cmdName != "(unknown)" {
		sc.cmdDurations[cmdName] = append(sc.cmdDurations[cmdName], duration)
	}
	if isWriteCommand(cmdName) {
		wc := formatWriteConcern(getMap(sa.Command, "writeConcern"))
		if wc == "" {
			wc = formatWriteConcern(sa.WriteConcern)
		}
		if wc == "" {
			wc = "(default)"
		}
		sc.wcDurations[wc] = append(sc.wcDurations[wc], duration)
	}

	queryHash := sa.QueryHash
	planCacheKey := sa.PlanCacheKey
	if queryHash != "" || planCacheKey != "" {
		shapeKey := queryHash + "|" + planCacheKey + "|" + ns + "|" + cmdName
		shape := sc.shapes[shapeKey]
		if shape == nil {
			shape = &slowShapeAccumulator{
				QueryHash:    queryHash,
				PlanCacheKey: planCacheKey,
				Namespace:    ns,
				Command:      cmdName,
			}
			sc.shapes[shapeKey] = shape
		}
		shape.Count++
		shape.TotalDurationMS += duration
		if duration > shape.MaxDurationMS {
			shape.MaxDurationMS = duration
		}
	}

	if duration > out.SlowQueries.MaxDurationMS {
		out.SlowQueries.MaxDurationMS = duration
	}
	if len(out.SlowQueries.Samples) < 10 {
		out.SlowQueries.Samples = append(out.SlowQueries.Samples, LogEvent{
			Time: ts, Severity: sev, Component: comp, Message: msg,
			Detail: fmt.Sprintf("%s %s duration=%dms", ns, cmdName, duration),
		})
	}
}

// collectLogEvent adds a warning or error sample to the summary.
func collectLogEvent(hdr logHeaderRaw, sev, comp, msg, ts string, out *LogSummary) {
	if (sev == "W" && len(out.Warnings) >= 20) || ((sev == "E" || sev == "F") && len(out.Errors) >= 20) {
		return
	}
	var attr map[string]any
	if len(hdr.Attr) > 0 {
		_ = json.Unmarshal(hdr.Attr, &attr)
	}
	event := LogEvent{Time: ts, Severity: sev, Component: comp, Message: msg, Detail: summarizeAttr(attr)}
	if sev == "W" {
		out.Warnings = append(out.Warnings, event)
	} else {
		out.Errors = append(out.Errors, event)
	}
}

func firstCommandName(command map[string]any) string {
	for _, name := range []string{
		"aggregate", "count", "distinct", "find", "findAndModify", "insert", "update", "delete",
		"getMore", "createIndexes", "dropIndexes", "explain", "mapReduce", "bulkWrite",
		"commitTransaction", "abortTransaction", "hello", "isMaster",
	} {
		if _, ok := command[name]; ok {
			return name
		}
	}
	for k := range command {
		switch k {
		case "$db", "$clusterTime", "lsid", "txnNumber", "writeConcern", "ordered", "query", "fields", "sort", "new", "pipeline", "cursor":
			continue
		}
		return k
	}
	return "(unknown)"
}

func summarizeAttr(attr map[string]any) string {
	for _, key := range []string{"error", "errmsg", "reason", "ns", "remote", "durationMillis"} {
		if v := str(attr[key]); v != "" {
			return trimString(v, 240)
		}
	}
	return ""
}

func addRecommendations(s *Summary) {
	s.Notes = append(s.Notes,
		"Recommended next sections: replication election timeline, connection/client IP breakdown, and sharding/balancer state when config server artifacts are present.",
	)
	if len(s.CommandLine.Errors) > 0 {
		s.Notes = append(s.Notes, "getCmdLineOpts did not return options; command-line options were recovered from mongod.log when available.")
	}
}

func shortHost(host string) string {
	host = strings.Split(host, ":")[0]
	return strings.Split(host, ".")[0]
}

func siteFromHost(host string) string {
	parts := strings.Split(strings.Split(host, ":")[0], ".")
	if len(parts) > 1 {
		return parts[1]
	}
	return ""
}

func number(s string) float64 {
	n, _ := strconv.ParseFloat(s, 64)
	return n
}

func intValue(v any) int64 {
	switch t := v.(type) {
	case int:
		return int64(t)
	case int64:
		return t
	case float64:
		return int64(t)
	case string:
		n, _ := strconv.ParseInt(t, 10, 64)
		return n
	default:
		n, _ := strconv.ParseInt(str(t), 10, 64)
		return n
	}
}

func percent(value, total int64) float64 {
	if total <= 0 {
		return 0
	}
	return float64(value) * 100 / float64(total)
}

func formatInt(v int64) string {
	sign := ""
	if v < 0 {
		sign = "-"
		v = -v
	}
	s := strconv.FormatInt(v, 10)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return sign + s
}

func formatBytes(v int64) string {
	units := []string{"B", "KiB", "MiB", "GiB", "TiB", "PiB"}
	value := float64(v)
	unit := 0
	for value >= 1024 && unit < len(units)-1 {
		value /= 1024
		unit++
	}
	return fmt.Sprintf("%.2f %s", value, units[unit])
}

func formatDurationMicros(v int64) string {
	if v == 0 {
		return "0s"
	}
	seconds := float64(v) / 1000000
	if seconds < 1 {
		return fmt.Sprintf("%.1f ms", float64(v)/1000)
	}
	return fmt.Sprintf("%.1f s", seconds)
}
