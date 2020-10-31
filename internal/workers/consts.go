package workers

import "time"

const (
	gracePeriod = 300 * time.Millisecond
	graceTimes  = 12
)
