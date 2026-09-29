package doctor

import "slices"

// Default is the full catalog of checks in report order.
func Default() []Check {
	return slices.Concat(
		nodeChecks(),
		iosChecks(),
		androidChecks(),
		goChecks(),
		claudeChecks(),
		shellChecks(),
		gitChecks(),
		globalsChecks(),
		networkChecks(),
		servicesChecks(),
		cacheChecks(),
		systemChecks(),
	)
}
