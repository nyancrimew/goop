package jobtracker

import (
	"sync"
	"sync/atomic"
	"time"
)

type JobTracker struct {
	activeWorkers int32
	queuedJobs    int32
	didWork       bool
	cond          *sync.Cond
	Queue         chan string
}

func Nap() {
	time.Sleep(40 * time.Millisecond)
}

func NewJobTracker() *JobTracker {
	return &JobTracker{
		cond:  sync.NewCond(&sync.Mutex{}),
		Queue: make(chan string, 999999), // TODO: dont create oversized queues, we should try to save memory; maybe read the channel docs again
	}
}

func (jt *JobTracker) AddJob(job string) {
	// TODO: can we discard empty jobs here?
	jt.cond.L.Lock()
	atomic.AddInt32(&jt.queuedJobs, 1)
	jt.Queue <- job
	jt.cond.L.Unlock()
}

func (jt *JobTracker) StartWork() {
	atomic.AddInt32(&jt.activeWorkers, 1)
}

func (jt *JobTracker) EndWork() {
	jt.didWork = true
	atomic.AddInt32(&jt.activeWorkers, -1)
	atomic.AddInt32(&jt.queuedJobs, -1)
}

func (jt *JobTracker) HasWork() bool {
	// TODO: didWork is a somewhat ugly workaround to ensure we dont exit before doing work at least once,
	// this will however result in locking up if we create a JobTracker but never queue any jobs
	hasWork := !jt.didWork || (atomic.LoadInt32(&jt.queuedJobs) > 0 && atomic.LoadInt32(&jt.activeWorkers) > 0)

	if !hasWork {
		jt.cond.Broadcast()
	}
	return hasWork
}

func (jt *JobTracker) Wait() {
	defer close(jt.Queue)

	jt.cond.L.Lock()
	for jt.HasWork() {
		jt.cond.Wait()
	}
}
