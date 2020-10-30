package stopandgo

import (
	"sync"
)

type StopAndGo struct {
	wg             sync.WaitGroup
	maxConcurrency int
	guard          chan struct{}
}

func NewStopAndGo(max int) *StopAndGo {
	return &StopAndGo{
		maxConcurrency: max,
		guard:          make(chan struct{}, max),
	}
}


func (s *StopAndGo) Add() {
	s.wg.Add(1)
	s.guard <- struct{}{}
}

func (s *StopAndGo) Done() {
	s.wg.Done()
	<-s.guard
}

func (s *StopAndGo) Wait() {
	s.wg.Wait()
	s.wg = sync.WaitGroup{}
}
