package collector

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

func Collect(inputDir, title string) (Summary, error) {
	info, err := os.Stat(inputDir)
	if err != nil {
		return Summary{}, err
	}
	if !info.IsDir() {
		return Summary{}, fmt.Errorf("%s is not a directory", inputDir)
	}

	s := Summary{
		Title:       title,
		InputDir:    inputDir,
		GeneratedAt: time.Now(),
	}

	files, err := filepath.Glob(filepath.Join(inputDir, "*"))
	if err != nil {
		return Summary{}, err
	}
	sort.Strings(files)

	// Only match exact filenames (no timestamp prefixes).
	targetSuffixes := []string{
		"getCmdLineOpts.out", "mongod.log", "rs_status.out", "rs_conf.out",
		"rs_printSlaveReplicationInfo.out", "rs_printReplicationInfo.out",
		"serverStatus.out", "currentOp.out", "getParameter.out", "lockinfo.out",
	}
	filesBySuffix := map[string]string{}
	for _, file := range files {
		base := filepath.Base(file)
		for _, suffix := range targetSuffixes {
			if base == suffix {
				filesBySuffix[suffix] = file
			}
		}
	}

	mongodLogPath := filesBySuffix["mongod.log"]
	mongodLogIdx := -1
	for _, file := range files {
		fi, err := os.Stat(file)
		if err != nil || fi.IsDir() {
			continue
		}
		fileInfo := FileInfo{Name: filepath.Base(file), Size: fi.Size()}
		if file == mongodLogPath {
			// Defer to parseLogFile, which already counts lines as it parses.
			mongodLogIdx = len(s.Files)
		} else {
			fileInfo.Lines, _ = countLines(file)
		}
		s.Files = append(s.Files, fileInfo)
	}

	optionsFromLog := map[string]string{}
	var slowByCommand map[string][]int
	if path := filesBySuffix["mongod.log"]; path != "" {
		logSummary, options, sbc, err := parseLogFile(path)
		if err != nil {
			s.Notes = append(s.Notes, "mongod.log parse failed: "+err.Error())
		}
		s.Log = logSummary
		optionsFromLog = options
		slowByCommand = sbc
		if mongodLogIdx >= 0 && mongodLogIdx < len(s.Files) {
			s.Files[mongodLogIdx].Lines = s.Log.Lines
		}
	}

	s.CommandLine = CommandLineSummary{Options: map[string]string{}}
	for k, v := range optionsFromLog {
		s.CommandLine.Options[k] = v
	}
	if len(s.CommandLine.Options) > 0 {
		s.CommandLine.Source = filepath.Base(filesBySuffix["mongod.log"])
	}

	// Build single snapshot from exact filenames.
	snap := Snapshot{
		CommandLine: CommandLineSummary{Options: map[string]string{}},
	}
	for k, v := range optionsFromLog {
		snap.CommandLine.Options[k] = v
	}
	if len(optionsFromLog) > 0 {
		snap.CommandLine.Source = filepath.Base(filesBySuffix["mongod.log"])
	}

	var serverStatusDoc map[string]any
	if path := filesBySuffix["serverStatus.out"]; path != "" {
		if doc, err := parseMongoShellFile(path); err == nil {
			snap.Server = parseServerSummary(doc)
			snap.WiredTiger = parseWiredTigerSummary(doc)
			serverStatusDoc = doc
		} else {
			s.Notes = append(s.Notes, "serverStatus.out parse failed: "+err.Error())
		}
	}
	if path := filesBySuffix["rs_status.out"]; path != "" {
		if doc, err := parseMongoShellFile(path); err == nil {
			snap.ReplicaSet = parseReplicaStatus(doc)
		} else {
			s.Notes = append(s.Notes, "rs_status.out parse failed: "+err.Error())
		}
	}
	if path := filesBySuffix["rs_conf.out"]; path != "" {
		if doc, err := parseMongoShellFile(path); err == nil {
			mergeReplicaConfig(&snap.ReplicaSet, doc)
		} else {
			s.Notes = append(s.Notes, "rs_conf.out parse failed: "+err.Error())
		}
	}
	if path := filesBySuffix["rs_printSlaveReplicationInfo.out"]; path != "" {
		mergeReplicaLag(&snap.ReplicaSet, path)
	}
	if path := filesBySuffix["rs_printReplicationInfo.out"]; path != "" {
		snap.ReplicaSet.Oplog = parseOplogSummary(path)
	}
	if path := filesBySuffix["getCmdLineOpts.out"]; path != "" {
		cli := parseCommandLineFile(path)
		if cli.Options == nil {
			cli.Options = map[string]string{}
		}
		for k, v := range snap.CommandLine.Options {
			if _, exists := cli.Options[k]; !exists {
				cli.Options[k] = v
			}
		}
		if cli.Source == "" {
			cli.Source = filepath.Base(path)
		}
		snap.CommandLine = cli
	}
	if path := filesBySuffix["currentOp.out"]; path != "" {
		if doc, err := parseMongoShellFile(path); err == nil {
			snap.CurrentOps = parseCurrentOps(doc)
		} else {
			s.Notes = append(s.Notes, "currentOp.out parse failed: "+err.Error())
		}
	}
	if path := filesBySuffix["getParameter.out"]; path != "" {
		if doc, err := parseMongoShellFile(path); err == nil {
			snap.Parameters = parseParameters(doc)
		} else {
			s.Notes = append(s.Notes, "getParameter.out parse failed: "+err.Error())
		}
	}
	snap.RED = parseRED(serverStatusDoc, s.Log, slowByCommand)
	s.Snapshots = append(s.Snapshots, snap)

	if path := filesBySuffix["lockinfo.out"]; path != "" {
		content, _ := os.ReadFile(path)
		s.RawArtifacts = append(s.RawArtifacts, RawArtifact{Title: filepath.Base(path), Content: trimString(string(content), 4000)})
	}

	addRecommendations(&s)
	return s, nil
}


func countLines(path string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	buf := make([]byte, 1024*1024)
	scanner.Buffer(buf, 8*1024*1024)
	count := 0
	for scanner.Scan() {
		count++
	}
	return count, scanner.Err()
}

func parseMongoShellFile(path string) (map[string]any, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	b = sanitizeTruncatedShellFields(b)
	obj, err := extractFirstObject(b)
	if err != nil {
		return nil, err
	}
	normalized := normalizeMongoShell(obj)
	normalized = normalizeSingleQuotedStrings(normalized)
	normalized = normalizeConcatenatedStrings(normalized)
	normalized = quoteBareObjectKeys(normalized)
	var doc map[string]any
	if err := json.Unmarshal(normalized, &doc); err != nil {
		var syn *json.SyntaxError
		if errors.As(err, &syn) {
			return nil, fmt.Errorf("%w near offset %d: %s", err, syn.Offset, syntaxErrorSnippet(normalized, syn.Offset))
		}
		return nil, err
	}
	return doc, nil
}

func sanitizeTruncatedShellFields(in []byte) []byte {
	lines := bytes.SplitAfter(in, []byte{'\n'})
	for i, line := range lines {
		s := string(line)
		if !strings.Contains(s, "$truncated") || !strings.Contains(s, ":") {
			continue
		}
		colon := strings.IndexByte(s, ':')
		if colon < 0 {
			continue
		}
		prefix := s[:colon+1]
		trailing := ""
		if strings.HasSuffix(strings.TrimSpace(s), ",") {
			trailing = ","
		}
		newline := ""
		if strings.HasSuffix(s, "\n") {
			newline = "\n"
		}
		lines[i] = []byte(prefix + " '[truncated]'" + trailing + newline)
	}
	return bytes.Join(lines, nil)
}

func normalizeConcatenatedStrings(in []byte) []byte {
	var out bytes.Buffer
	out.Grow(len(in))

	for i := 0; i < len(in); {
		if in[i] != '"' {
			out.WriteByte(in[i])
			i++
			continue
		}

		segRaw, segDecoded, next, ok := parseJSONStringLiteral(in, i)
		if !ok {
			out.WriteByte(in[i])
			i++
			continue
		}

		combined := segDecoded
		j := next
		merged := false
		for {
			wsStart := j
			for wsStart < len(in) && (in[wsStart] == ' ' || in[wsStart] == '\t' || in[wsStart] == '\n' || in[wsStart] == '\r') {
				wsStart++
			}
			if wsStart >= len(in) || in[wsStart] != '+' {
				break
			}
			wsAfterPlus := wsStart + 1
			for wsAfterPlus < len(in) && (in[wsAfterPlus] == ' ' || in[wsAfterPlus] == '\t' || in[wsAfterPlus] == '\n' || in[wsAfterPlus] == '\r') {
				wsAfterPlus++
			}
			if wsAfterPlus >= len(in) || in[wsAfterPlus] != '"' {
				break
			}
			nextRaw, nextDecoded, nextPos, ok := parseJSONStringLiteral(in, wsAfterPlus)
			if !ok {
				break
			}
			_ = nextRaw
			combined += nextDecoded
			j = nextPos
			merged = true
		}

		if merged {
			out.WriteString(strconv.Quote(combined))
		} else {
			out.Write(segRaw)
		}
		i = j
	}

	return out.Bytes()
}

func parseJSONStringLiteral(in []byte, start int) (raw []byte, decoded string, next int, ok bool) {
	if start >= len(in) || in[start] != '"' {
		return nil, "", start, false
	}
	i := start + 1
	escaped := false
	for i < len(in) {
		c := in[i]
		if escaped {
			escaped = false
			i++
			continue
		}
		if c == '\\' {
			escaped = true
			i++
			continue
		}
		if c == '"' {
			raw = in[start : i+1]
			decodedVal, err := strconv.Unquote(string(raw))
			if err != nil {
				return nil, "", start, false
			}
			return raw, decodedVal, i + 1, true
		}
		i++
	}
	return nil, "", start, false
}
func normalizeSingleQuotedStrings(in []byte) []byte {
	var out bytes.Buffer
	out.Grow(len(in) + len(in)/16)

	var quote byte
	escaped := false
	for i := 0; i < len(in); i++ {
		c := in[i]
		if quote == '"' {
			out.WriteByte(c)
			if escaped {
				escaped = false
			} else if c == '\\' {
				escaped = true
			} else if c == '"' {
				quote = 0
			}
			continue
		}
		if quote == '\'' {
			if escaped {
				switch c {
				case '"':
					out.WriteString(`\"`)
				case '\'':
					out.WriteByte('\'')
				case '\\':
					out.WriteString(`\\`)
				default:
					out.WriteByte('\\')
					out.WriteByte(c)
				}
				escaped = false
				continue
			}
			if c == '\\' {
				escaped = true
				continue
			}
			if c == '\'' {
				out.WriteByte('"')
				quote = 0
				continue
			}
			if c == '"' {
				out.WriteString(`\"`)
				continue
			}
			out.WriteByte(c)
			continue
		}

		switch c {
		case '"':
			quote = '"'
			out.WriteByte(c)
		case '\'':
			quote = '\''
			out.WriteByte('"')
		default:
			out.WriteByte(c)
		}
	}
	return out.Bytes()
}

func quoteBareObjectKeys(in []byte) []byte {
	var out bytes.Buffer
	out.Grow(len(in) + len(in)/16)
	var quote byte
	escaped := false
	stack := make([]byte, 0, 16)
	expectKey := make([]bool, 0, 16)

	isIdentStart := func(c byte) bool {
		return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '_' || c == '$'
	}
	isIdentPart := func(c byte) bool {
		return isIdentStart(c) || (c >= '0' && c <= '9')
	}

	for i := 0; i < len(in); {
		c := in[i]
		if quote != 0 {
			out.WriteByte(c)
			switch {
			case escaped:
				escaped = false
			case c == '\\':
				escaped = true
			case c == quote:
				quote = 0
			}
			i++
			continue
		}

		switch c {
		case '"', '\'':
			quote = c
			out.WriteByte(c)
			i++
			continue
		case '{':
			stack = append(stack, '{')
			expectKey = append(expectKey, true)
			out.WriteByte(c)
			i++
			continue
		case '[':
			stack = append(stack, '[')
			expectKey = append(expectKey, false)
			out.WriteByte(c)
			i++
			continue
		case '}':
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
				expectKey = expectKey[:len(expectKey)-1]
			}
			out.WriteByte(c)
			i++
			continue
		case ']':
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
				expectKey = expectKey[:len(expectKey)-1]
			}
			out.WriteByte(c)
			i++
			continue
		case ':':
			if len(stack) > 0 && stack[len(stack)-1] == '{' {
				expectKey[len(expectKey)-1] = false
			}
			out.WriteByte(c)
			i++
			continue
		case ',':
			if len(stack) > 0 && stack[len(stack)-1] == '{' {
				expectKey[len(expectKey)-1] = true
			}
			out.WriteByte(c)
			i++
			continue
		}

		if len(stack) > 0 && stack[len(stack)-1] == '{' && expectKey[len(expectKey)-1] && isIdentStart(c) {
			start := i
			j := i + 1
			for j < len(in) && isIdentPart(in[j]) {
				j++
			}
			k := j
			for k < len(in) && (in[k] == ' ' || in[k] == '\t' || in[k] == '\r' || in[k] == '\n') {
				k++
			}
			if k < len(in) && in[k] == ':' {
				out.WriteByte('"')
				out.Write(in[start:j])
				out.WriteByte('"')
				i = j
				continue
			}
		}

		out.WriteByte(c)
		i++
	}
	return out.Bytes()
}

func extractFirstObject(b []byte) ([]byte, error) {
	// Skip leading whitespace before searching for the opening brace.
	i := 0
	for i < len(b) && (b[i] == ' ' || b[i] == '\t' || b[i] == '\n' || b[i] == '\r') {
		i++
	}
	start := bytes.IndexByte(b[i:], '{')
	if start < 0 {
		return nil, fmt.Errorf("no object found")
	}
	start += i
	depth := 0
	var quote byte
	escaped := false
	for i := start; i < len(b); i++ {
		c := b[i]
		if quote != 0 {
			if escaped {
				escaped = false
				continue
			}
			if c == '\\' {
				escaped = true
				continue
			}
			if c == quote {
				quote = 0
			}
			continue
		}
		if c == '"' || c == '\'' {
			quote = c
			continue
		}
		switch c {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return b[start : i+1], nil
			}
		}
	}
	return nil, fmt.Errorf("unterminated object")
}

type mongoShellRule struct {
	nameB   []byte
	replace func(args string) string
}

var mongoShellRules = func() []mongoShellRule {
	rules := []struct {
		name    string
		replace func(string) string
	}{
		{"NumberLong", mongoFunctionValue},
		{"Long", mongoFunctionValue},
		{"NumberDecimal", mongoFunctionValue},
		{"ISODate", mongoFunctionValue},
		{"ObjectId", mongoFunctionValue},
		{"UUID", mongoFunctionValue},
		{"Timestamp", timestampReplacement},
		{"Binary.createFromBase64", binaryFromBase64Replacement},
		{"BinData", func(string) string { return `"BinData"` }},
		{"HexData", func(string) string { return `"HexData"` }},
		{"DBRef", func(string) string { return `"DBRef"` }},
		{"RegExp", func(string) string { return `"RegExp"` }},
	}
	out := make([]mongoShellRule, len(rules))
	for i, r := range rules {
		out[i] = mongoShellRule{nameB: []byte(r.name), replace: r.replace}
	}
	return out
}()

func normalizeMongoShell(in []byte) []byte {
	var buf bytes.Buffer
	buf.Grow(len(in))
	normalizeMongoShellInto(&buf, in)
	return buf.Bytes()
}

func normalizeMongoShellInto(out *bytes.Buffer, in []byte) {
	i := 0
	var quote byte
	escaped := false
	for i < len(in) {
		c := in[i]
		if quote != 0 {
			out.WriteByte(c)
			switch {
			case escaped:
				escaped = false
			case c == '\\':
				escaped = true
			case c == quote:
				quote = 0
			}
			i++
			continue
		}
		if c == '"' || c == '\'' {
			quote = c
			out.WriteByte(c)
			i++
			continue
		}
		ruleIdx := -1
		for ri := range mongoShellRules {
			r := &mongoShellRules[ri]
			n := len(r.nameB)
			if i+n < len(in) && in[i+n] == '(' && bytes.HasPrefix(in[i:], r.nameB) {
				ruleIdx = ri
				break
			}
		}
		if ruleIdx < 0 {
			out.WriteByte(c)
			i++
			continue
		}
		r := &mongoShellRules[ruleIdx]
		start := i + len(r.nameB) + 1
		end := start
		depth := 1
		var q byte
		ce := false
		for end < len(in) {
			cc := in[end]
			if q != 0 {
				if ce {
					ce = false
				} else if cc == '\\' {
					ce = true
				} else if cc == q {
					q = 0
				}
				end++
				continue
			}
			if cc == '"' || cc == '\'' {
				q = cc
				end++
				continue
			}
			if cc == '(' {
				depth++
			} else if cc == ')' {
				depth--
				if depth == 0 {
					break
				}
			}
			end++
		}
		if end >= len(in) {
			out.Write(in[i:])
			return
		}
		argsBytes := in[start:end]
		var args string
		if bytes.IndexByte(argsBytes, '(') >= 0 {
			var argsBuf bytes.Buffer
			argsBuf.Grow(len(argsBytes))
			normalizeMongoShellInto(&argsBuf, argsBytes)
			args = argsBuf.String()
		} else {
			args = string(argsBytes)
		}
		out.WriteString(r.replace(args))
		i = end + 1
	}
}

func timestampReplacement(args string) string {
	parts := splitMongoArgs(args)
	if len(parts) == 1 {
		part := strings.TrimSpace(parts[0])
		if strings.HasPrefix(part, "{") && strings.HasSuffix(part, "}") {
			return part
		}
		return `"` + part + `"`
	}
	if len(parts) == 2 {
		return fmt.Sprintf(`{"t":%s,"i":%s}`, strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]))
	}
	return `"` + strings.TrimSpace(args) + `"`
}

func mongoFunctionValue(args string) string {
	args = strings.TrimSpace(args)
	if strings.HasPrefix(args, `"`) && strings.HasSuffix(args, `"`) {
		return args
	}
	if strings.HasPrefix(args, `'`) && strings.HasSuffix(args, `'`) {
		args = strings.TrimPrefix(args, `'`)
		args = strings.TrimSuffix(args, `'`)
		args = strings.ReplaceAll(args, `\`, `\\`)
		args = strings.ReplaceAll(args, `"`, `\"`)
		args = strings.ReplaceAll(args, `\'`, `'`)
		return `"` + args + `"`
	}
	return args
}

func binaryFromBase64Replacement(args string) string {
	parts := splitMongoArgs(args)
	if len(parts) == 0 {
		return `""`
	}
	return mongoFunctionValue(parts[0])
}

func splitMongoArgs(args string) []string {
	var parts []string
	start := 0
	depthParen := 0
	depthBrace := 0
	depthBracket := 0
	var quote byte
	escaped := false

	for i := 0; i < len(args); i++ {
		c := args[i]
		if quote != 0 {
			if escaped {
				escaped = false
				continue
			}
			if c == '\\' {
				escaped = true
				continue
			}
			if c == quote {
				quote = 0
			}
			continue
		}

		switch c {
		case '"', '\'':
			quote = c
		case '(':
			depthParen++
		case ')':
			if depthParen > 0 {
				depthParen--
			}
		case '{':
			depthBrace++
		case '}':
			if depthBrace > 0 {
				depthBrace--
			}
		case '[':
			depthBracket++
		case ']':
			if depthBracket > 0 {
				depthBracket--
			}
		case ',':
			if depthParen == 0 && depthBrace == 0 && depthBracket == 0 {
				parts = append(parts, args[start:i])
				start = i + 1
			}
		}
	}
	parts = append(parts, args[start:])

	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func syntaxErrorSnippet(b []byte, offset int64) string {
	if offset < 1 {
		offset = 1
	}
	pos := int(offset - 1)
	if pos > len(b) {
		pos = len(b)
	}
	start := pos - 40
	if start < 0 {
		start = 0
	}
	end := pos + 40
	if end > len(b) {
		end = len(b)
	}
	snippet := string(b[start:end])
	snippet = strings.ReplaceAll(snippet, "\n", " ")
	snippet = strings.ReplaceAll(snippet, "\r", " ")
	snippet = strings.TrimSpace(snippet)
	return fmt.Sprintf("...%s...", snippet)
}

func getMap(m map[string]any, key string) map[string]any {
	v, _ := m[key].(map[string]any)
	return v
}

func getSlice(m map[string]any, key string) []any {
	v, _ := m[key].([]any)
	return v
}

func str(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case float64:
		if t == float64(int64(t)) {
			return strconv.FormatInt(int64(t), 10)
		}
		return strconv.FormatFloat(t, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(t)
	default:
		b, _ := json.Marshal(t)
		return string(b)
	}
}

func dotted(doc map[string]any, path ...string) string {
	var cur any = doc
	for _, p := range path {
		m, ok := cur.(map[string]any)
		if !ok {
			return ""
		}
		cur = m[p]
	}
	return str(cur)
}

func flatten(prefix string, v any, out map[string]string) {
	switch t := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			next := k
			if prefix != "" {
				next = prefix + "." + k
			}
			flatten(next, t[k], out)
		}
	case []any:
		out[prefix] = fmt.Sprintf("%d item(s)", len(t))
	default:
		out[prefix] = str(t)
	}
}

func sortedCounts(counts map[string]int, limit int) []Count {
	items := make([]Count, 0, len(counts))
	for k, v := range counts {
		if k == "" {
			k = "(blank)"
		}
		items = append(items, Count{Key: k, Count: v})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Count == items[j].Count {
			return items[i].Key < items[j].Key
		}
		return items[i].Count > items[j].Count
	})
	if limit > 0 && len(items) > limit {
		return items[:limit]
	}
	return items
}

func trimString(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	return s[:limit] + "\n... truncated ..."
}

func readLines(path string, fn func(string)) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	r := bufio.NewReaderSize(f, 1024*1024)
	for {
		line, err := r.ReadString('\n')
		if len(line) > 0 {
			fn(strings.TrimRight(line, "\r\n"))
		}
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
	}
}
