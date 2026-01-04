package cmd

// Options enthält alle Optionen für plan und apply
type Options struct {
	Artifacts      []string // Spezifische Artefakte die ausgewählt werden
	RollbackCommit string   // Commit für Rollback
	DryRun         bool     // Nur anzeigen, nicht ausführen
}
