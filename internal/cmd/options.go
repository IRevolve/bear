package cmd

// Options contains all options for plan and apply
type Options struct {
	Artifacts []string // Specific artifacts to select
	PinCommit string   // Commit to pin artifact(s) to
	Force     bool     // Ignore pinned artifacts
	NoCommit  bool     // Disable automatic commit after apply (default: commit enabled)
	Verbose   bool     // Show step output even on success
}
