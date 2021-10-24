package jobtracker

import (
	"container/list"
	"math"
	"sync"
	"sync/atomic"
	"time"
)

type JobTracker struct {
	worker         Worker
	maxConcurrency int32
	activeWorkers  int32
	queuedJobs     int32
	naps           int32
	napper         Napper
	started        bool
	cond           *sync.Cond
	queue          chan string
	send           chan string
}

type Worker func(jt *JobTracker, job string, context Context)
type Napper func(naps, activeWorkers int32)
type Context interface{}

// DefaultNapper will sleep a minimum of 15 milliseconds per nap, but increasingly longer the more often workers go idle
// this ensures that idle workers do not take up too much cpu time, and should prevent accidental early exits
func DefaultNapper(naps int32, activeWorkers int32) {
	ratio := float64(naps) / float64(activeWorkers)
	n := int(math.Ceil(ratio))
	if n < 1 {
		n = 1
	}
	time.Sleep(time.Duration(n) * 15 * time.Millisecond)
}

func NewJobTracker(worker Worker, maxConcurrency int32, napper Napper) *JobTracker {
	jt := &JobTracker{
		worker:         worker,
		maxConcurrency: maxConcurrency,
		cond:           sync.NewCond(&sync.Mutex{}),
		napper:         napper,
		queue:          make(chan string, 1),
		send:           make(chan string, 1),
	}
	go jt.manageQueue()
	return jt
}

func (jt *JobTracker) AddJob(job string) {
	if job == "" {
		return
	}
	atomic.AddInt32(&jt.queuedJobs, 1)
	jt.send <- job
}

func (jt *JobTracker) AddJobs(jobs ...string) {
	for _, job := range jobs {
		jt.AddJob(job)
	}
}

func (jt *JobTracker) StartAndWait(context Context) {
	numWorkers := int(min32(atomic.LoadInt32(&jt.queuedJobs), jt.maxConcurrency))
	for w := 0; w < numWorkers; w++ {
		go workRoutine(jt, jt.worker, context)
	}
	jt.started = true
	jt.cond.L.Lock()
	for jt.hasWork() {
		jt.cond.Wait()
	}
}

func (jt *JobTracker) manageQueue() {
	defer close(jt.queue)
	defer close(jt.send)
	queue := list.New()
	for jt.hasWork() {
		if front := queue.Front(); front == nil {
			value, ok := <-jt.send
			if ok {
				queue.PushBack(value)
			}
		} else {
			select {
			case jt.queue <- front.Value.(string):
				queue.Remove(front)
			case value, ok := <-jt.send:
				if ok {
					queue.PushBack(value)
				}
			}
		}
	}
}

func (jt *JobTracker) startWork() {
	atomic.AddInt32(&jt.activeWorkers, 1)
}

func (jt *JobTracker) nap() {
	jt.napper(atomic.LoadInt32(&jt.naps), atomic.LoadInt32(&jt.activeWorkers))
}

func (jt *JobTracker) endWork() {
	atomic.AddInt32(&jt.activeWorkers, -1)
	atomic.AddInt32(&jt.queuedJobs, -1)
}

func (jt *JobTracker) hasWork() bool {
	hasWork := !jt.started || atomic.LoadInt32(&jt.queuedJobs) > 0 || atomic.LoadInt32(&jt.activeWorkers) > 0
	if !hasWork {
		jt.cond.Broadcast()
	}
	return hasWork
}

func workRoutine(jt *JobTracker, worker Worker, context Context) {
	for {
		select {
		case job := <-jt.queue:
			jt.startWork()
			worker(jt, job, context)
			jt.endWork()
		default:
			if !jt.hasWork() {
				return
			}
			jt.nap()
		}
	}
}

func min32(x, y int32) int32 {
	if x < y {
		return x
	}
	return y
}
