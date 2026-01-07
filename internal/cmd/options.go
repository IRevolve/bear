package cmd

// Options contains all options for plan and apply
type Options struct {
	Artifacts []string // Specific artifacts to select
	PinCommit string   // Commit to pin artifact(s) to
	Validate  bool     // Run validation commands (lint, test)
	Force     bool     // Ignore pinned artifacts
	Commit    bool     // Automatically commit after apply
}
