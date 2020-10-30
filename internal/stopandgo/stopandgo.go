// a thing that at least in theory limits concurrency, but i honestly have no clue if it works
package stopandgo

import (
	"sync"
	"sync/atomic"
)

type StopAndGo struct {
	wg sync.WaitGroup
	maxConcurrency int
	n int32
}

func NewStopAndGo(max int) *StopAndGo {
	return &StopAndGo{
		maxConcurrency: max,
	}
}

func (s *StopAndGo) check() {
	if int(atomic.LoadInt32(&s.n)) >= s.maxConcurrency {
		s.Wait()
	}
}

func (s *StopAndGo) AddN(n int32) {
	atomic.AddInt32(&s.n, n)
	s.wg.Add(int(n))
	// check on a new, separate goroutine so we don't lock everything up
	go s.check()
}

func (s *StopAndGo) Add() {
	s.AddN(1)
}

func (s *StopAndGo) Done () {
	atomic.AddInt32(&s.n, -1)
	s.wg.Done()
}

func (s *StopAndGo) Wait() {
	s.wg.Wait()
	s.wg = sync.WaitGroup{}
}
