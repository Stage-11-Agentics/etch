package archive

// ApplyQuarterForTest applies one quarter plan through the same transaction
// Archive uses — lets tests apply a deliberately stale plan to exercise the
// all-or-nothing guard.
func ApplyQuarterForTest(opts Options, q QuarterPlan) error {
	return archiveQuarter(opts, q)
}
