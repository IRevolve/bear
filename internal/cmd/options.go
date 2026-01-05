package cmd

// Options contains all options for plan and apply
type Options struct {
	Artifacts      []string // Specific artifacts to select
	RollbackCommit string   // Commit for rollback
	DryRun         bool     // Only display, don't execute
	Force          bool     // Ignore pinned artifacts
	Commit         bool     // Automatically commit after apply
}
