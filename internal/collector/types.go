package collector

import "time"

// Snapshot holds parsed data from a single collection of MongoDB diagnostic files.
// Only one snapshot is created per run, built from exact filename matches.
type Snapshot struct {
	Server      ServerSummary
	WiredTiger  WiredTigerSummary
	CommandLine CommandLineSummary
	ReplicaSet  ReplicaSetSummary
	CurrentOps  CurrentOpSummary
	RED         REDSummary
	Parameters  ParameterSummary
}

type Summary struct {
	Title        string
	InputDir     string
	GeneratedAt  time.Time
	Files        []FileInfo
	Log          LogSummary        // Static: parsed once from mongod.log
	CommandLine  CommandLineSummary // Static: merged from getCmdLineOpts.out + log options
	Snapshots    []Snapshot         // Single snapshot from exact filename matches
	RawArtifacts []RawArtifact
	Notes        []string
}

type REDSummary struct {
	Available         bool
	UptimeSec         float64
	LogWindowSec      float64
	Commands          []REDCommand
	LatencyCategories []REDLatencyCategory
	Process           REDProcess
	Observations      []HealthObservation
}

type REDCommand struct {
	Name           string
	Total          int64
	Failed         int64
	RequestsPerSec float64
	ErrorsPerSec   float64
	ErrorPercent   float64
	SlowCount      int
	SlowP95MS      int
	SlowP99MS      int
}

type REDLatencyCategory struct {
	Category       string
	Ops            int64
	RequestsPerSec float64
	MeanLatencyMS  float64
}

type REDProcess struct {
	UserAsserts        int64
	UserAssertsPerSec  float64
	LogErrors          int
	LogErrorsPerSec    float64
	DuplicateKey       int
	DuplicateKeyPerSec float64
}

type FileInfo struct {
	Name  string
	Size  int64
	Lines int
}

type ServerSummary struct {
	Host                 string
	Version              string
	Process              string
	PID                  string
	Uptime               string
	LocalTime            string
	FCV                  string
	ConnectionsCurrent   string
	ConnectionsActive    string
	ConnectionsAvailable string
	DefaultReadConcern   string
	DefaultWriteConcern  string
	UserAssertions       string
	CatalogCollections   string
}

type WiredTigerSummary struct {
	Available              bool
	CacheBytes             string
	MaxCacheBytes          string
	CacheUsedPercent       float64
	TrackedDirtyBytes      string
	DirtyCachePercent      float64
	DirtyUsedPercent       float64
	PagesHeld              string
	PagesRead              string
	PagesWritten           string
	PagesRequested         string
	ModifiedPagesEvicted   string
	UnmodifiedPagesEvicted string
	AppThreadEvictedPages  string
	ForcedEvictionSelected string
	ForcedEvictionFailed   string
	CheckpointBlocked      string
	HazardPointerBlocked   string
	EvictionWalksAbandoned string
	EvictionNoProgress     string
	UnableToReachGoal      string
	AggressiveMode         string
	CacheWaitOps           string
	CacheWaitTime          string
	CacheTimeouts          string
	CapacityWaitEviction   string
	Observations           []HealthObservation
}

type HealthObservation struct {
	Level   string
	Message string
}

type CommandLineSummary struct {
	Source  string
	Options map[string]string
	Errors  []string
}

type ReplicaSetSummary struct {
	Name                  string
	Date                  string
	Term                  string
	MyState               string
	MajorityVoteCount     string
	WriteMajorityCount    string
	VotingMembersCount    string
	WritableVotingMembers string
	Primary               string
	Members               []ReplicaMember
	Oplog                 OplogSummary
	ConfigSettings        map[string]string
}

type ReplicaMember struct {
	ID             string
	Name           string
	HostShort      string
	Site           string
	State          string
	Health         string
	Uptime         string
	PingMS         string
	SyncSourceHost string
	ReplicationLag string
	OptimeDate     string
	Priority       string
	Votes          string
	ArbiterOnly    string
	Hidden         string
	Self           bool
	Children       []int
}

type OplogSummary struct {
	ConfiguredSize string
	Window         string
	FirstEvent     string
	LastEvent      string
	Now            string
}

type CurrentOpSummary struct {
	Total          int
	Active         int
	WaitingForLock int
	WaitingForFlow int
	Ops            []Count
	Namespaces     []Count
	Drivers        []Count
	Longest        []OperationSample
}

type OperationSample struct {
	Op          string
	Namespace   string
	Client      string
	SecsRunning string
	Description string
}

type LogSummary struct {
	Lines             int
	Start             string
	End               string
	SeverityCounts    []Count
	ComponentCounts   []Count
	MessageCounts     []Count
	SlowQueries       SlowQuerySummary
	DuplicateKeyCount int
	Warnings          []LogEvent
	Errors            []LogEvent
}

type SlowQuerySummary struct {
	Count               int
	MaxDurationMS       int
	AvgDurationMS       float64
	NamespaceCount      []Count
	CommandCount        []Count
	Shapes              []SlowQueryShape
	WriteConcernLatency []WriteConcernLatency
	Samples             []LogEvent
}

type WriteConcernLatency struct {
	Concern       string
	Count         int
	MinDurationMS int
	MaxDurationMS int
	AvgDurationMS float64
	P50DurationMS int
	P90DurationMS int
	P95DurationMS int
	P99DurationMS int
}

type SlowQueryShape struct {
	QueryHash     string
	PlanCacheKey  string
	Namespace     string
	Command       string
	Count         int
	MaxDurationMS int
	AvgDurationMS float64
}

type LogEvent struct {
	Time      string
	Severity  string
	Component string
	Message   string
	Detail    string
}

type ParameterSummary struct {
	Highlights map[string]string
}

type RawArtifact struct {
	Title   string
	Content string
}

type Count struct {
	Key   string
	Count int
}
