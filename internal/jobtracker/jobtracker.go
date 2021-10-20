package jobtracker

import (
	"container/list"
	"sync"
	"sync/atomic"
	"time"
)

type JobTracker struct {
	activeWorkers int32
	queuedJobs    int32
	didWork       bool
	stop          bool
	cond          *sync.Cond
	Queue         chan string
	send          chan string
}

func Nap() {
	time.Sleep(40 * time.Millisecond)
}

func NewJobTracker() *JobTracker {
	jt := &JobTracker{
		cond:  sync.NewCond(&sync.Mutex{}),
		Queue: make(chan string, 1),
		send:  make(chan string, 1),
	}
	go jt.manageQueue()
	return jt
}

func (jt *JobTracker) manageQueue() {
	queue := list.New()
	for jt.HasWork() {
		if front := queue.Front(); front == nil {
			value, ok := <-jt.send
			if ok {
				queue.PushBack(value)
			}
		} else {
			select {
			case jt.Queue <- front.Value.(string):
				queue.Remove(front)
			case value, ok := <-jt.send:
				if ok {
					queue.PushBack(value)
				}
			}
		}
	}

	close(jt.Queue)
	close(jt.send)
}

func (jt *JobTracker) AddJob(job string) {
	// TODO: can we discard empty jobs here?
	atomic.AddInt32(&jt.queuedJobs, 1)
	jt.send <- job
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
	if jt.stop {
		return false
	}
	// TODO: didWork is a somewhat ugly workaround to ensure we dont exit before doing work at least once,
	// this will however result in locking up if we create a JobTracker but never queue any jobs
	hasWork := !jt.didWork || (atomic.LoadInt32(&jt.queuedJobs) > 0 && atomic.LoadInt32(&jt.activeWorkers) > 0)

	if !hasWork {
		jt.cond.Broadcast()
	}
	return hasWork
}

func (jt *JobTracker) KillIfNoJobs() bool {
	if atomic.LoadInt32(&jt.queuedJobs) == 0 {
		jt.stop = true
		return true
	}
	return false
}

func (jt *JobTracker) QueuedJobs() int32 {
	return atomic.LoadInt32(&jt.queuedJobs)
}

func (jt *JobTracker) Wait() {
	jt.KillIfNoJobs()
	jt.cond.L.Lock()
	for jt.HasWork() {
		jt.cond.Wait()
	}
}
