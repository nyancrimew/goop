package jobtracker

import (
	"container/list"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/deletescape/goop/internal/utils"
)

type JobTracker struct {
	activeWorkers int32
	queuedJobs    int32
	naps          int32
	didWork       bool
	stop          bool
	cond          *sync.Cond
	Queue         chan string
	send          chan string
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
	defer close(jt.Queue)
	defer close(jt.send)
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
}

func (jt *JobTracker) AddJob(job string) {
	if job == "" {
		return
	}
	atomic.AddInt32(&jt.queuedJobs, 1)
	jt.send <- job
}

func (jt *JobTracker) StartWork() {
	atomic.AddInt32(&jt.activeWorkers, 1)
}

func (jt *JobTracker) Nap() {
	ratio := float64(atomic.AddInt32(&jt.naps, 1)) / float64(atomic.LoadInt32(&jt.activeWorkers))
	n := utils.MaxInt(int(math.Ceil(ratio)), 1)
	time.Sleep(time.Duration(n) * 15 * time.Millisecond)
}

func (jt *JobTracker) EndWork() {
	jt.didWork = true
	atomic.AddInt32(&jt.activeWorkers, -1)
	atomic.AddInt32(&jt.queuedJobs, -1)
}

func (jt *JobTracker) HasWork() bool {
	hasWork := !jt.stop && (!jt.didWork || (atomic.LoadInt32(&jt.queuedJobs) > 0 && atomic.LoadInt32(&jt.activeWorkers) > 0))

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
	if !jt.KillIfNoJobs() {
		jt.cond.L.Lock()
		for jt.HasWork() {
			jt.cond.Wait()
		}
	}
}
