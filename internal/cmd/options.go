package cmd

import "context"

// Options contains the options shared by plan, apply, and validate. Each
// command reads only the fields relevant to it; see individual field comments.
type Options struct {
	Context     context.Context
	GitRemote   string   // Git remote for lock-file publication; apply only
	GitBranch   string   // Git branch for lock-file publication; apply only
	Environment string   // Deployment environment: required by plan, optional by validate (injects ENVIRONMENT only), unused by apply (read from the saved plan)
	Artifacts   []string // Specific artifacts to select; used by plan and validate
	PinCommit   string   // Commit to pin artifact(s) to; plan only
	Force       bool     // Ignore pinned artifacts; plan only, apply never reads it
	NoCommit    bool     // Disable automatic commit after apply (default: commit enabled)
	Concurrency int      // Max parallel jobs (default: 10); used by validate and apply, not plan (plan runs nothing)
	Verbose     bool     // Show step output even on success; used by validate and apply, not plan
}
