package collector

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// extractFirstObject
// ---------------------------------------------------------------------------

func TestExtractFirstObject(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{
			name:  "simple object",
			input: `{"a":1}`,
			want:  `{"a":1}`,
		},
		{
			name:  "leading spaces",
			input: `   {"a":1}`,
			want:  `{"a":1}`,
		},
		{
			name:  "leading tabs",
			input: "\t\t{\"a\":1}",
			want:  `{"a":1}`,
		},
		{
			name:  "leading newlines",
			input: "\n\n{\"a\":1}",
			want:  `{"a":1}`,
		},
		{
			name:  "mixed leading whitespace",
			input: " \t\n {\"a\":1}",
			want:  `{"a":1}`,
		},
		{
			name:  "nested objects",
			input: `{"a":{"b":2}}`,
			want:  `{"a":{"b":2}}`,
		},
		{
			name:  "string containing braces",
			input: `{"a":"{hello} {world}"}`,
			want:  `{"a":"{hello} {world}"}`,
		},
		{
			name:  "string with escaped quotes",
			input: `{"a":"he\"llo"}`,
			want:  `{"a":"he\"llo"}`,
		},
		{
			name:  "multiple objects returns first",
			input: `{"a":1} {"b":2}`,
			want:  `{"a":1}`,
		},
		{
			name:    "no object returns error",
			input:   `not json at all`,
			wantErr: true,
		},
		{
			name:    "empty input",
			input:   ``,
			wantErr: true,
		},
		{
			name:    "whitespace only",
			input:   "  \t\n  ",
			wantErr: true,
		},
		{
			name:  "empty object",
			input: `{}`,
			want:  `{}`,
		},
		{
			name:  "simple leading whitespace mixed with content",
			input: "  {\"x\":[1,2,3]}",
			want:  `{"x":[1,2,3]}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := extractFirstObject([]byte(tt.input))
			if tt.wantErr {
				if err == nil {
					t.Errorf("extractFirstObject() error = nil, wantErr true")
				}
				return
			}
			if err != nil {
				t.Errorf("extractFirstObject() error = %v", err)
				return
			}
			if string(got) != tt.want {
				t.Errorf("extractFirstObject() = %q, want %q", string(got), tt.want)
			}
		})
	}
}

func TestExtractFirstObject_Unterminated(t *testing.T) {
	// Unterminated object should return an error, not panic.
	_, err := extractFirstObject([]byte(`{"a":1`))
	if err == nil {
		t.Error("extractFirstObject() expected error for unterminated object")
	}
}

// ---------------------------------------------------------------------------
// normalizeMongoShell
// ---------------------------------------------------------------------------

func TestNormalizeMongoShell_PlainJSON(t *testing.T) {
	input := []byte(`{"a":1,"b":"hello","c":true,"d":null}`)
	got := string(normalizeMongoShell(input))
	want := `{"a":1,"b":"hello","c":true,"d":null}`
	if got != want {
		t.Errorf("normalizeMongoShell() = %q, want %q", got, want)
	}
}

func TestNormalizeMongoShell_NumberLong(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{`{"a":NumberLong(1)}`, `{"a":1}`},
		{`{"a":NumberLong("12345678901234567")}`, `{"a":"12345678901234567"}`},
		{`{"a":NumberLong( -5 )}`, `{"a":-5}`},
		{`{"a":Long("604559")}`, `{"a":"604559"}`},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := string(normalizeMongoShell([]byte(tt.input)))
			if got != tt.want {
				t.Errorf("normalizeMongoShell() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNormalizeMongoShell_NumberDecimal(t *testing.T) {
	input := `{"a":NumberDecimal("123.45")}`
	got := string(normalizeMongoShell([]byte(input)))
	want := `{"a":"123.45"}`
	if got != want {
		t.Errorf("normalizeMongoShell() = %q, want %q", got, want)
	}
}

func TestNormalizeMongoShell_ISODate(t *testing.T) {
	input := `{"a":ISODate("2024-01-01T00:00:00Z")}`
	got := string(normalizeMongoShell([]byte(input)))
	want := `{"a":"2024-01-01T00:00:00Z"}`
	if got != want {
		t.Errorf("normalizeMongoShell() = %q, want %q", got, want)
	}
}

func TestNormalizeMongoShell_ObjectId(t *testing.T) {
	input := `{"a":ObjectId("507f1f77bcf86cd799439011")}`
	got := string(normalizeMongoShell([]byte(input)))
	want := `{"a":"507f1f77bcf86cd799439011"}`
	if got != want {
		t.Errorf("normalizeMongoShell() = %q, want %q", got, want)
	}
}

func TestNormalizeMongoShell_Timestamp(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{`{"a":Timestamp(1234567890, 1)}`, `{"a":{"t":1234567890,"i":1}}`},
		{`{"a":Timestamp(0, 0)}`, `{"a":{"t":0,"i":0}}`},
		{`{"a":Timestamp({ t: 1779310340, i: 11 })}`, `{"a":{ t: 1779310340, i: 11 }}`},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := string(normalizeMongoShell([]byte(tt.input)))
			if got != tt.want {
				t.Errorf("normalizeMongoShell() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSplitMongoArgs(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{name: "simple pair", in: `1,2`, want: []string{"1", "2"}},
		{name: "object arg", in: `{ t: 1779310340, i: 11 }`, want: []string{`{ t: 1779310340, i: 11 }`}},
		{name: "nested constructors", in: `NumberLong(100), NumberLong(2)`, want: []string{"NumberLong(100)", "NumberLong(2)"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := splitMongoArgs(tt.in)
			if len(got) != len(tt.want) {
				t.Fatalf("splitMongoArgs() length = %d, want %d (%v)", len(got), len(tt.want), got)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("splitMongoArgs()[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestNormalizeMongoShell_BinData(t *testing.T) {
	input := `{"a":BinData(0,"abcd")}`
	got := string(normalizeMongoShell([]byte(input)))
	want := `{"a":"BinData"}`
	if got != want {
		t.Errorf("normalizeMongoShell() = %q, want %q", got, want)
	}
}

func TestNormalizeMongoShell_HexData(t *testing.T) {
	input := `{"a":HexData(0,"abcd")}`
	got := string(normalizeMongoShell([]byte(input)))
	want := `{"a":"HexData"}`
	if got != want {
		t.Errorf("normalizeMongoShell() = %q, want %q", got, want)
	}
}

func TestNormalizeMongoShell_DBRef(t *testing.T) {
	input := `{"a":DBRef("collection", ObjectId("507f1f77bcf86cd799439011"))}`
	got := string(normalizeMongoShell([]byte(input)))
	want := `{"a":"DBRef"}`
	if got != want {
		t.Errorf("normalizeMongoShell() = %q, want %q", got, want)
	}
}

func TestNormalizeMongoShell_RegExp(t *testing.T) {
	input := `{"a":RegExp("pattern","i")}`
	got := string(normalizeMongoShell([]byte(input)))
	want := `{"a":"RegExp"}`
	if got != want {
		t.Errorf("normalizeMongoShell() = %q, want %q", got, want)
	}
}

func TestNormalizeMongoShell_UUID(t *testing.T) {
	input := `{"a":UUID("some-uuid-value")}`
	got := string(normalizeMongoShell([]byte(input)))
	want := `{"a":"some-uuid-value"}`
	if got != want {
		t.Errorf("normalizeMongoShell() = %q, want %q", got, want)
	}
}

func TestNormalizeMongoShell_BinaryCreateFromBase64(t *testing.T) {
	input := `{"hash":Binary.createFromBase64("dXLF5kfj5Za6m76sCA0e4Q==",0)}`
	got := string(normalizeMongoShell([]byte(input)))
	want := `{"hash":"dXLF5kfj5Za6m76sCA0e4Q=="}`
	if got != want {
		t.Errorf("normalizeMongoShell() = %q, want %q", got, want)
	}
}

func TestNormalizeMongoShell_NestedFunctions(t *testing.T) {
	// Nested function inside another function's args.
	input := `{"a":Timestamp(NumberLong(100), NumberLong(2))}`
	got := string(normalizeMongoShell([]byte(input)))
	want := `{"a":{"t":100,"i":2}}`
	if got != want {
		t.Errorf("normalizeMongoShell() = %q, want %q", got, want)
	}
}

func TestNormalizeMongoShell_MultipleFunctions(t *testing.T) {
	input := `{"created":ISODate("2024-01-01T00:00:00Z"),"count":NumberLong(42),"ts":Timestamp(100,0)}`
	got := string(normalizeMongoShell([]byte(input)))
	want := `{"created":"2024-01-01T00:00:00Z","count":42,"ts":{"t":100,"i":0}}`
	if got != want {
		t.Errorf("normalizeMongoShell() = %q, want %q", got, want)
	}
}

func TestNormalizeConcatenatedStrings(t *testing.T) {
	input := `{"msg":"line1\n" + "line2"}`
	got := string(normalizeConcatenatedStrings([]byte(input)))
	want := `{"msg":"line1\nline2"}`
	if got != want {
		t.Errorf("normalizeConcatenatedStrings() = %q, want %q", got, want)
	}
}

func TestNormalizeMongoShell_RealWorldLike(t *testing.T) {
	// Simulates a realistic serverStatus or rs.status document snippet.
	input := `{"set":"rs0","date":ISODate("2024-06-15T10:30:00Z"),"myState":NumberLong(1),"members":[{"_id":NumberLong(0),"name":"host1:27017","stateStr":"PRIMARY"}]}`
	got := string(normalizeMongoShell([]byte(input)))
	want := `{"set":"rs0","date":"2024-06-15T10:30:00Z","myState":1,"members":[{"_id":0,"name":"host1:27017","stateStr":"PRIMARY"}]}`
	if got != want {
		t.Errorf("normalizeMongoShell() = %q, want %q", got, want)
	}
}

func TestQuoteBareObjectKeys(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "simple bare keys",
			input: `{host:"db1",pid:123}`,
			want:  `{"host":"db1","pid":123}`,
		},
		{
			name:  "nested object and array",
			input: `{host:"db1",connections:{current:12,active:3},tags:[{k:"a",v:1}]}`,
			want:  `{"host":"db1","connections":{"current":12,"active":3},"tags":[{"k":"a","v":1}]}`,
		},
		{
			name:  "preserve quoted keys",
			input: `{"host":"db1",pid:123}`,
			want:  `{"host":"db1","pid":123}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := string(quoteBareObjectKeys([]byte(tt.input)))
			if got != tt.want {
				t.Errorf("quoteBareObjectKeys() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseMongoShellFile_BareKeys(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "serverStatus.out")
	content := `{ host: "db1", version: "7.0.0", pid: NumberLong(123), connections: { current: NumberLong(12), active: NumberLong(3), available: NumberLong(100) } }`
	if err := os.WriteFile(p, []byte(content), 0644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	doc, err := parseMongoShellFile(p)
	if err != nil {
		t.Fatalf("parseMongoShellFile() error = %v", err)
	}
	if str(doc["host"]) != "db1" {
		t.Errorf("host = %q, want %q", str(doc["host"]), "db1")
	}
	if str(doc["version"]) != "7.0.0" {
		t.Errorf("version = %q, want %q", str(doc["version"]), "7.0.0")
	}
	if str(doc["pid"]) != "123" {
		t.Errorf("pid = %q, want %q", str(doc["pid"]), "123")
	}
}

func TestParseMongoShellFile_SingleQuotedValues(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "serverStatus.out")
	content := `{ host: 'db1', version: '7.0.0', process: 'mongod', pid: NumberLong('123'), connections: { current: NumberLong('12') } }`
	if err := os.WriteFile(p, []byte(content), 0644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	doc, err := parseMongoShellFile(p)
	if err != nil {
		t.Fatalf("parseMongoShellFile() error = %v", err)
	}
	if str(doc["host"]) != "db1" {
		t.Errorf("host = %q, want %q", str(doc["host"]), "db1")
	}
	if str(doc["pid"]) != "123" {
		t.Errorf("pid = %q, want %q", str(doc["pid"]), "123")
	}
}

func TestParseMongoShellFile_SyntaxErrorContext(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "serverStatus.out")
	content := `{ host: 'db1', version: ??? }`
	if err := os.WriteFile(p, []byte(content), 0644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	_, err := parseMongoShellFile(p)
	if err == nil {
		t.Fatal("expected parseMongoShellFile error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "near offset") {
		t.Errorf("expected syntax error offset context, got %q", msg)
	}
	if !strings.Contains(msg, "...") {
		t.Errorf("expected snippet marker in error, got %q", msg)
	}
}

func TestSanitizeTruncatedShellFields(t *testing.T) {
	in := "{\n  '$truncated': '{ foo: \"bar...'\n}\n"
	got := string(sanitizeTruncatedShellFields([]byte(in)))
	want := "{\n  '$truncated': '[truncated]'\n}\n"
	if got != want {
		t.Fatalf("sanitizeTruncatedShellFields() = %q, want %q", got, want)
	}
}

func TestParseMongoShellFile_TruncatedFieldMalformedContent(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "currentOp.out")
	content := "{\n" +
		"  inprog: [\n" +
		"    {\n" +
		"      command: {\n" +
		"        '$truncated': '{ $truncated: \"{ aggregate: \"routeRule\", platform: \"Java/Chainguard/21.0.12+-cgr-...'\n" +
		"      },\n" +
		"      op: 'getmore'\n" +
		"    }\n" +
		"  ],\n" +
		"  ok: 1\n" +
		"}\n"
	if err := os.WriteFile(p, []byte(content), 0644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	doc, err := parseMongoShellFile(p)
	if err != nil {
		t.Fatalf("parseMongoShellFile() error = %v", err)
	}
	if str(doc["ok"]) != "1" {
		t.Fatalf("ok = %q, want %q", str(doc["ok"]), "1")
	}
}

// ---------------------------------------------------------------------------
// percentileInt
// ---------------------------------------------------------------------------

func TestPercentileInt(t *testing.T) {
	tests := []struct {
		name   string
		sorted []int
		p      int
		want   int
	}{
		{"single element, p=50", []int{42}, 50, 42},
		{"single element, p=0", []int{42}, 0, 42},
		{"single element, p=100", []int{42}, 100, 42},
		{"two elements, p=50", []int{10, 20}, 50, 10},
		{"two elements, p=90", []int{10, 20}, 90, 10}, // nearest-rank gives sorted[0]
		{"two elements, p=100", []int{10, 20}, 100, 20},
		{"five elements, p=50", []int{1, 2, 3, 4, 5}, 50, 3},
		{"five elements, p=90", []int{1, 2, 3, 4, 5}, 90, 4},
		{"five elements, p=95", []int{1, 2, 3, 4, 5}, 95, 4},
		{"five elements, p=99", []int{1, 2, 3, 4, 5}, 99, 4},
		{"five elements, p=0", []int{1, 2, 3, 4, 5}, 0, 1},
		{"five elements, p=100", []int{1, 2, 3, 4, 5}, 100, 5},
		{"ten elements, p=50", []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}, 50, 4},
		{"ten elements, p=90", []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}, 90, 8},
		{"ten elements, p=95", []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}, 95, 8},
		{"ten elements, p=99", []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}, 99, 8},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := percentileInt(tt.sorted, tt.p)
			if got != tt.want {
				t.Errorf("percentileInt(%v, %d) = %d, want %d", tt.sorted, tt.p, got, tt.want)
			}
		})
	}
}

func TestPercentileInt_Empty(t *testing.T) {
	got := percentileInt([]int{}, 50)
	if got != 0 {
		t.Errorf("percentileInt(empty, 50) = %d, want 0", got)
	}
}

func TestPercentileInt_NegativePercentile(t *testing.T) {
	got := percentileInt([]int{10, 20, 30}, -1)
	if got != 10 {
		t.Errorf("percentileInt(... , -1) = %d, want 10", got)
	}
}

func TestPercentileInt_Over100(t *testing.T) {
	got := percentileInt([]int{10, 20, 30}, 200)
	if got != 30 {
		t.Errorf("percentileInt(... , 200) = %d, want 30", got)
	}
}

// ---------------------------------------------------------------------------
// computeWriteConcernLatency
// ---------------------------------------------------------------------------

func TestComputeWriteConcernLatency_Empty(t *testing.T) {
	got := computeWriteConcernLatency(map[string][]int{})
	if len(got) != 0 {
		t.Errorf("computeWriteConcernLatency(empty) = %v, want empty", got)
	}
}

func TestComputeWriteConcernLatency_Single(t *testing.T) {
	durations := map[string][]int{
		"w=majority": {10, 20, 30, 40, 100},
	}
	got := computeWriteConcernLatency(durations)
	if len(got) != 1 {
		t.Fatalf("expected 1 result, got %d", len(got))
	}
	r := got[0]
	if r.Concern != "w=majority" {
		t.Errorf("Concern = %q, want %q", r.Concern, "w=majority")
	}
	if r.Count != 5 {
		t.Errorf("Count = %d, want 5", r.Count)
	}
	if r.MinDurationMS != 10 {
		t.Errorf("Min = %d, want 10", r.MinDurationMS)
	}
	if r.MaxDurationMS != 100 {
		t.Errorf("Max = %d, want 100", r.MaxDurationMS)
	}
	if r.AvgDurationMS != 40.0 {
		t.Errorf("Avg = %f, want 40.0", r.AvgDurationMS)
	}
	if r.P50DurationMS != 30 {
		t.Errorf("P50 = %d, want 30", r.P50DurationMS)
	}
	if r.P90DurationMS != 40 {
		t.Errorf("P90 = %d, want 40", r.P90DurationMS)
	}
	// nearest-rank: (95*4)/100 = 3, sorted[3] = 40
	if r.P95DurationMS != 40 {
		t.Errorf("P95 = %d, want 40", r.P95DurationMS)
	}
	// nearest-rank: (99*4)/100 = 3, sorted[3] = 40
	if r.P99DurationMS != 40 {
		t.Errorf("P99 = %d, want 40", r.P99DurationMS)
	}
}

func TestComputeWriteConcernLatency_MultipleConcerns(t *testing.T) {
	durations := map[string][]int{
		"w=1":           {5, 10, 15},
		"w=majority, j=true": {50, 100, 150},
	}
	got := computeWriteConcernLatency(durations)
	if len(got) != 2 {
		t.Fatalf("expected 2 results, got %d", len(got))
	}
	// Results should be sorted by count descending (both have 3, so by concern name).
	if got[0].Concern > got[1].Concern {
		got[0], got[1] = got[1], got[0]
	}
	for _, r := range got {
		if r.Count != 3 {
			t.Errorf("Concern %q: Count = %d, want 3", r.Concern, r.Count)
		}
	}
}

func TestComputeWriteConcernLatency_SkipsEmpty(t *testing.T) {
	durations := map[string][]int{
		"w=1": {},
		"w=2": {10, 20},
	}
	got := computeWriteConcernLatency(durations)
	if len(got) != 1 {
		t.Errorf("expected 1 result (skipping empty), got %d", len(got))
	}
}

// ---------------------------------------------------------------------------
// formatWriteConcern
// ---------------------------------------------------------------------------

func TestFormatWriteConcern(t *testing.T) {
	tests := []struct {
		name string
		wc   map[string]any
		want string
	}{
		{"empty", map[string]any{}, ""},
		{"only w", map[string]any{"w": "majority"}, "w=majority"},
		{"w and j", map[string]any{"w": "1", "j": true}, "w=1, j=true"},
		{"w, j, wtimeout", map[string]any{"w": "2", "j": false, "wtimeout": int64(5000)}, "w=2, j=false, wtimeout=5000"},
		{"wtimeout zero omitted", map[string]any{"w": "1", "wtimeout": int64(0)}, "w=1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatWriteConcern(tt.wc)
			if got != tt.want {
				t.Errorf("formatWriteConcern(%v) = %q, want %q", tt.wc, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// isWriteCommand
// ---------------------------------------------------------------------------

func TestIsWriteCommand(t *testing.T) {
	writes := []string{"insert", "update", "delete", "findAndModify", "bulkWrite", "commitTransaction"}
	reads := []string{"find", "aggregate", "count", "distinct", "getMore", "hello", "isMaster"}
	for _, name := range writes {
		if !isWriteCommand(name) {
			t.Errorf("isWriteCommand(%q) = false, want true", name)
		}
	}
	for _, name := range reads {
		if isWriteCommand(name) {
			t.Errorf("isWriteCommand(%q) = true, want false", name)
		}
	}
}

// ---------------------------------------------------------------------------
// firstCommandName
// ---------------------------------------------------------------------------

func TestFirstCommandName(t *testing.T) {
	tests := []struct {
		name    string
		command map[string]any
		want    string
	}{
		{"find", map[string]any{"find": "collection", "filter": map[string]any{}}, "find"},
		{"aggregate", map[string]any{"aggregate": "collection", "pipeline": []any{}}, "aggregate"},
		{"insert", map[string]any{"insert": "collection", "documents": []any{}}, "insert"},
		{"unknown key", map[string]any{"customCmd": "val"}, "customCmd"},
		{"only metadata keys", map[string]any{"$db": "test", "lsid": map[string]any{}, "txnNumber": int64(1)}, "(unknown)"},
		{"empty map", map[string]any{}, "(unknown)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := firstCommandName(tt.command)
			if got != tt.want {
				t.Errorf("firstCommandName(%v) = %q, want %q", tt.command, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// shortHost / siteFromHost
// ---------------------------------------------------------------------------

func TestShortHost(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"host1.example.com:27017", "host1"},
		{"host1.example.com", "host1"},
		{"localhost", "localhost"},
		{"", ""},
		{"host1.example.com:1234:5678", "host1"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := shortHost(tt.input)
			if got != tt.want {
				t.Errorf("shortHost(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestSiteFromHost(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"host1.example.com:27017", "example"},
		{"host1.example.com", "example"},
		{"localhost", ""},
		{"", ""},
		{"host1", ""},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := siteFromHost(tt.input)
			if got != tt.want {
				t.Errorf("siteFromHost(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// str helper
// ---------------------------------------------------------------------------

func TestStr(t *testing.T) {
	tests := []struct {
		input any
		want  string
	}{
		{nil, ""},
		{string("hello"), "hello"},
		{int64(42), "42"},
		{float64(3.14), "3.14"},
		{float64(100.0), "100"},
		{true, "true"},
		{false, "false"},
		{[]any{1, 2}, "[1,2]"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := str(tt.input)
			if got != tt.want {
				t.Errorf("str(%v) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// intValue helper
// ---------------------------------------------------------------------------

func TestIntValue(t *testing.T) {
	tests := []struct {
		input any
		want  int64
	}{
		{int(42), 42},
		{int64(42), 42},
		{float64(42.0), 42},
		{float64(42.7), 42}, // truncation
		{string("42"), 42},
		{nil, 0},
	}
	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			got := intValue(tt.input)
			if got != tt.want {
				t.Errorf("intValue(%v) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// trimString
// ---------------------------------------------------------------------------

func TestTrimString(t *testing.T) {
	tests := []struct {
		input string
		limit int
		want  string
	}{
		{"hello", 10, "hello"},
		{"hello world", 5, "hello\n... truncated ..."},
		{"", 5, ""},
		{"abc", 3, "abc"},
		{"abcd", 3, "abc\n... truncated ..."},
	}
	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			got := trimString(tt.input, tt.limit)
			if got != tt.want {
				t.Errorf("trimString(%q, %d) = %q, want %q", tt.input, tt.limit, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// sortedCounts
// ---------------------------------------------------------------------------

func TestSortedCounts(t *testing.T) {
	tests := []struct {
		name  string
		input map[string]int
		limit int
		want  []string // ordered keys
	}{
		{"empty", map[string]int{}, 0, nil},
		{"single", map[string]int{"a": 1}, 0, []string{"a"}},
		{"multiple sorted by count desc", map[string]int{"a": 5, "b": 10, "c": 1}, 0, []string{"b", "a", "c"}},
		{"tie broken by key", map[string]int{"b": 5, "a": 5}, 0, []string{"a", "b"}},
		{"limit", map[string]int{"a": 5, "b": 10, "c": 1}, 2, []string{"b", "a"}},
		{"blank key", map[string]int{"": 5}, 0, []string{"(blank)"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sortedCounts(tt.input, tt.limit)
			if len(got) != len(tt.want) {
				t.Fatalf("sortedCounts() = %d items, want %d", len(got), len(tt.want))
			}
			for i, key := range tt.want {
				if got[i].Key != key {
					t.Errorf("sortedCounts()[%d].Key = %q, want %q", i, got[i].Key, key)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// computeLogWindowSeconds
// ---------------------------------------------------------------------------

func TestComputeLogWindowSeconds(t *testing.T) {
	tests := []struct {
		name  string
		start string
		end   string
		want  float64
	}{
		{"empty start", "", "2024-01-01T00:01:00Z", 0},
		{"empty end", "2024-01-01T00:00:00Z", "", 0},
		{"both empty", "", "", 0},
		{"RFC3339 60s", "2024-01-01T00:00:00Z", "2024-01-01T00:01:00Z", 60},
		{"RFC3339Nano", "2024-01-01T00:00:00.000Z", "2024-01-01T00:01:00.500Z", 60.5},
		{"same time is zero", "2024-01-01T00:00:00Z", "2024-01-01T00:00:00Z", 0},
		{"end before start is zero", "2024-01-01T00:01:00Z", "2024-01-01T00:00:00Z", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := computeLogWindowSeconds(tt.start, tt.end)
			if got != tt.want {
				t.Errorf("computeLogWindowSeconds(%q, %q) = %f, want %f", tt.start, tt.end, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// formatInt, formatBytes, formatDurationMicros
// ---------------------------------------------------------------------------

func TestFormatInt(t *testing.T) {
	tests := []struct {
		input int64
		want  string
	}{
		{0, "0"},
		{1, "1"},
		{1000, "1,000"},
		{1000000, "1,000,000"},
		{-1000, "-1,000"},
		{123456789, "123,456,789"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := formatInt(tt.input)
			if got != tt.want {
				t.Errorf("formatInt(%d) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		input int64
		want  string
	}{
		{0, "0.00 B"},
		{500, "500.00 B"},
		{1024, "1.00 KiB"},
		{1536, "1.50 KiB"},
		{1048576, "1.00 MiB"},
		{1073741824, "1.00 GiB"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := formatBytes(tt.input)
			if got != tt.want {
				t.Errorf("formatBytes(%d) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestFormatDurationMicros(t *testing.T) {
	tests := []struct {
		input int64
		want  string
	}{
		{0, "0s"},
		{500, "0.5 ms"},
		{1000, "1.0 ms"},
		{1000000, "1.0 s"},
		{1500000, "1.5 s"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := formatDurationMicros(tt.input)
			if got != tt.want {
				t.Errorf("formatDurationMicros(%d) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// health observations
// ---------------------------------------------------------------------------

func TestWiredTigerObservations_CacheLevels(t *testing.T) {
	base := WiredTigerSummary{Available: true, MaxCacheBytes: "10.00 GiB"}

	t.Run("low cache usage", func(t *testing.T) {
		base.CacheUsedPercent = 30
		obs := wiredTigerObservations(base, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0)
		okCount := 0
		for _, o := range obs {
			if o.Level == "ok" {
				okCount++
			}
		}
		if okCount == 0 {
			t.Errorf("expected at least one 'ok' observation for low cache usage")
		}
	})

	t.Run("warning cache usage", func(t *testing.T) {
		base.CacheUsedPercent = 80
		obs := wiredTigerObservations(base, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0)
		found := false
		for _, o := range obs {
			if o.Level == "warn" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected 'warn' observation for 80%% cache usage")
		}
	})

	t.Run("bad cache usage", func(t *testing.T) {
		base.CacheUsedPercent = 96
		obs := wiredTigerObservations(base, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0)
		found := false
		for _, o := range obs {
			if o.Level == "bad" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected 'bad' observation for 96%% cache usage")
		}
	})
}

func TestWiredTigerObservations_DirtyLevels(t *testing.T) {
	base := WiredTigerSummary{Available: true, MaxCacheBytes: "10.00 GiB", CacheUsedPercent: 50}

	t.Run("low dirty cache", func(t *testing.T) {
		base.DirtyCachePercent = 2
		obs := wiredTigerObservations(base, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0)
		found := false
		for _, o := range obs {
			if o.Level == "ok" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected 'ok' observation for 2%% dirty cache")
		}
	})

	t.Run("warning dirty cache", func(t *testing.T) {
		base.DirtyCachePercent = 10
		obs := wiredTigerObservations(base, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0)
		found := false
		for _, o := range obs {
			if o.Level == "warn" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected 'warn' observation for 10%% dirty cache")
		}
	})

	t.Run("bad dirty cache", func(t *testing.T) {
		base.DirtyCachePercent = 25
		obs := wiredTigerObservations(base, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0)
		found := false
		for _, o := range obs {
			if o.Level == "bad" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected 'bad' observation for 25%% dirty cache")
		}
	})
}

func TestWiredTigerObservations_EvictionPressure(t *testing.T) {
	base := WiredTigerSummary{Available: true, MaxCacheBytes: "10.00 GiB", CacheUsedPercent: 50, DirtyCachePercent: 2}

	t.Run("aggressive mode triggers bad", func(t *testing.T) {
		obs := wiredTigerObservations(base, 0, 0, 0, 0, 0, 1, 1, 0, 0, 0, 0)
		found := false
		for _, o := range obs {
			if o.Level == "bad" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected 'bad' observation for aggressive mode")
		}
	})

	t.Run("forced eviction with high failure rate", func(t *testing.T) {
		obs := wiredTigerObservations(base, 100, 50, 0, 0, 0, 0, 0, 0, 0, 0, 0)
		found := false
		for _, o := range obs {
			if o.Level == "bad" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected 'bad' observation for high forced-eviction failure rate")
		}
	})

	t.Run("forced eviction with low failure rate", func(t *testing.T) {
		obs := wiredTigerObservations(base, 100, 10, 0, 0, 0, 0, 0, 0, 0, 0, 0)
		foundWarn := false
		for _, o := range obs {
			if o.Level == "warn" {
				foundWarn = true
				break
			}
		}
		if !foundWarn {
			t.Errorf("expected 'warn' observation for forced eviction with low failure")
		}
	})

	t.Run("checkpoint blocked", func(t *testing.T) {
		obs := wiredTigerObservations(base, 0, 0, 5, 0, 0, 0, 0, 0, 0, 0, 0)
		found := false
		for _, o := range obs {
			if o.Level == "warn" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected 'warn' observation for checkpoint blocked evictions")
		}
	})

	t.Run("cache waits recorded", func(t *testing.T) {
		obs := wiredTigerObservations(base, 0, 0, 0, 0, 0, 0, 0, 10, 100000, 0, 50000)
		found := false
		for _, o := range obs {
			if o.Level == "warn" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected 'warn' observation for cache waits")
		}
	})
}

func TestRedObservations(t *testing.T) {
	base := REDSummary{Available: true}

	t.Run("high error rate", func(t *testing.T) {
		r := base
		r.Commands = []REDCommand{
			{Name: "insert", Total: 100, Failed: 10, ErrorPercent: 10},
		}
		obs := redObservations(r)
		if len(obs) == 0 {
			t.Fatal("expected observations for high error rate")
		}
		if obs[0].Level != "bad" {
			t.Errorf("expected 'bad', got %q", obs[0].Level)
		}
	})

	t.Run("moderate error rate", func(t *testing.T) {
		r := base
		r.Commands = []REDCommand{
			{Name: "find", Total: 1000, Failed: 15, ErrorPercent: 1.5},
		}
		obs := redObservations(r)
		if len(obs) == 0 {
			t.Fatal("expected observations for moderate error rate")
		}
		if obs[0].Level != "warn" {
			t.Errorf("expected 'warn', got %q", obs[0].Level)
		}
	})

	t.Run("low error rate no observation", func(t *testing.T) {
		r := base
		r.Commands = []REDCommand{
			{Name: "find", Total: 1000, Failed: 2, ErrorPercent: 0.2},
		}
		obs := redObservations(r)
		for _, o := range obs {
			if o.Level == "bad" || o.Level == "warn" {
				t.Errorf("unexpected observation for low error rate: %s: %s", o.Level, o.Message)
			}
		}
	})

	t.Run("high mean latency", func(t *testing.T) {
		r := base
		r.LatencyCategories = []REDLatencyCategory{
			{Category: "reads", Ops: 1000, MeanLatencyMS: 150},
		}
		obs := redObservations(r)
		if len(obs) == 0 {
			t.Fatal("expected observations for high latency")
		}
		if obs[0].Level != "warn" {
			t.Errorf("expected 'warn', got %q", obs[0].Level)
		}
	})

	t.Run("no issues", func(t *testing.T) {
		r := base
		r.Commands = []REDCommand{
			{Name: "find", Total: 1000, Failed: 0, ErrorPercent: 0},
		}
		r.LatencyCategories = []REDLatencyCategory{
			{Category: "reads", Ops: 1000, MeanLatencyMS: 5},
		}
		r.Process = REDProcess{LogErrorsPerSec: 0.01}
		obs := redObservations(r)
		if len(obs) != 0 {
			t.Errorf("expected 0 observations for healthy state, got %d", len(obs))
		}
	})
}

func TestRedObservations_HighLogErrors(t *testing.T) {
	r := REDSummary{
		Available:   true,
		UptimeSec:   3600,
		LogWindowSec: 300,
		Process: REDProcess{
			LogErrors:       500,
			LogErrorsPerSec: 1.67,
		},
	}
	obs := redObservations(r)
	found := false
	for _, o := range obs {
		if o.Level == "warn" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected 'warn' observation for high log errors per second")
	}
}

// ---------------------------------------------------------------------------
// scanLogLine
// ---------------------------------------------------------------------------

func TestScanLogLine(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantOK   bool
		wantSev  string
		wantComp string
		wantMsg  string
	}{
		{
			name:   "standard log line",
			input:  `{"t":{"$date":"2024-06-15T10:00:00.000Z"},"s":"I","c":"COMMAND","id":123,"ctx":"conn1","msg":"Slow query","attr":{"ns":"test.collection","durationMillis":500}}`,
			wantOK: true, wantSev: "I", wantComp: "COMMAND", wantMsg: "Slow query",
		},
		{
			name:   "warning line",
			input:  `{"t":{"$date":"2024-06-15T10:00:01.000Z"},"s":"W","c":"STORAGE","id":456,"ctx":"conn2","msg":"Connection terminated"}`,
			wantOK: true, wantSev: "W", wantComp: "STORAGE", wantMsg: "Connection terminated",
		},
		{
			name:   "error line",
			input:  `{"t":{"$date":"2024-06-15T10:00:02.000Z"},"s":"E","c":"ACCESS","id":789,"ctx":"conn3","msg":"Authentication failed","attr":{"error":"Unauthorized"}}`,
			wantOK: true, wantSev: "E", wantComp: "ACCESS", wantMsg: "Authentication failed",
		},
		{
			name:   "non-JSON line",
			input:  `2024-06-15T10:00:00.000+00:00 I COMMAND  [conn1] some text log`,
			wantOK: false,
		},
		{
			name:   "empty input",
			input:  ``,
			wantOK: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hdr, ok := scanLogLine([]byte(tt.input))
			if ok != tt.wantOK {
				t.Errorf("scanLogLine() ok = %v, want %v", ok, tt.wantOK)
				return
			}
			if !ok {
				return
			}
			if string(hdr.Severity) != tt.wantSev {
				t.Errorf("Severity = %q, want %q", string(hdr.Severity), tt.wantSev)
			}
			if string(hdr.Component) != tt.wantComp {
				t.Errorf("Component = %q, want %q", string(hdr.Component), tt.wantComp)
			}
			if string(hdr.Message) != tt.wantMsg {
				t.Errorf("Message = %q, want %q", string(hdr.Message), tt.wantMsg)
			}
		})
	}
}

func TestScanLogLine_Attr(t *testing.T) {
	input := `{"t":{"$date":"2024-01-01T00:00:00.000Z"},"s":"I","c":"NETWORK","id":1,"ctx":"conn1","msg":"received","attr":{"remote":"10.0.0.1:12345","connectionId":5}}`
	hdr, ok := scanLogLine([]byte(input))
	if !ok {
		t.Fatal("scanLogLine() returned false")
	}
	if len(hdr.Attr) == 0 {
		t.Fatal("Attr is empty")
	}
	// Verify attr contains the expected JSON by parsing it.
	if string(hdr.Attr) != `{"remote":"10.0.0.1:12345","connectionId":5}` {
		t.Errorf("Attr = %q, want %q", string(hdr.Attr), `{"remote":"10.0.0.1:12345","connectionId":5}`)
	}
}

func TestScanLogLine_EscapedStrings(t *testing.T) {
	input := `{"t":{"$date":"2024-01-01T00:00:00.000Z"},"s":"I","c":"NETWORK","id":1,"ctx":"conn1","msg":"line with \"quotes\" inside"}`
	hdr, ok := scanLogLine([]byte(input))
	if !ok {
		t.Fatal("scanLogLine() returned false")
	}
	if string(hdr.Message) != `line with \"quotes\" inside` {
		t.Errorf("Message = %q, want %q", string(hdr.Message), `line with \"quotes\" inside`)
	}
	// The backslash is kept because the scanner returns raw string content from JSON.
}

func TestScanLogLine_MissingFields(t *testing.T) {
	// t and s present, others missing
	input := `{"t":{"$date":"2024-01-01T00:00:00.000Z"},"s":"I"}`
	hdr, ok := scanLogLine([]byte(input))
	if !ok {
		t.Fatal("scanLogLine() returned false for minimal valid line")
	}
	if string(hdr.Severity) != "I" {
		t.Errorf("Severity = %q, want 'I'", string(hdr.Severity))
	}
	if string(hdr.Component) != "" {
		t.Errorf("Component should be empty, got %q", string(hdr.Component))
	}
	if string(hdr.Message) != "" {
		t.Errorf("Message should be empty, got %q", string(hdr.Message))
	}
}

func TestScanLogLine_NoTimestamp(t *testing.T) {
	// t is present but without $date (someone passed a non-standard format)
	// The extractDateField should return nil, but scanLogLine should still return ok.
	input := `{"t":12345,"s":"I","c":"NETWORK","id":1,"ctx":"conn1","msg":"hello"}`
	hdr, ok := scanLogLine([]byte(input))
	if !ok {
		t.Fatal("scanLogLine() returned false when t is not an object")
	}
	if hdr.Timestamp != nil {
		t.Errorf("Timestamp should be nil for non-object t, got %q", string(hdr.Timestamp))
	}
}

// ---------------------------------------------------------------------------
// flatten helper
// ---------------------------------------------------------------------------

func TestFlatten(t *testing.T) {
	out := map[string]string{}
	flatten("", map[string]any{
		"a": "1",
		"b": map[string]any{
			"c": "2",
			"d": "3",
		},
		"e": []any{1, 2, 3},
	}, out)
	expected := map[string]string{
		"a":   "1",
		"b.c": "2",
		"b.d": "3",
		"e":   "3 item(s)",
	}
	if len(out) != len(expected) {
		t.Errorf("flatten() produced %d entries, want %d: %v", len(out), len(expected), out)
	}
	for k, v := range expected {
		if out[k] != v {
			t.Errorf("flatten()[%q] = %q, want %q", k, out[k], v)
		}
	}
}

// ---------------------------------------------------------------------------
// time-based: ensure time is working
// ---------------------------------------------------------------------------

func TestGeneratedAt(t *testing.T) {
	// Very simple test: ensure time is within reasonable bounds.
	now := time.Now()
	s := Summary{GeneratedAt: now}
	if s.GeneratedAt.IsZero() {
		t.Error("GeneratedAt should not be zero")
	}
}
