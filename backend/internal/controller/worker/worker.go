package worker

import "github.com/riverqueue/river"

// The count matters: River refuses to fetch jobs without a registered kind.
func Register(_ *river.Workers) int {
	return 0
}
