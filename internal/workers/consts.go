package workers

import "time"

const (
	gracePeriod = 350 * time.Millisecond
	graceTimes  = 15
)
