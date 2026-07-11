// Package workerpool provides a goroutine pool for connection handling
package workerpool

import (
	"sync"
	"sync/atomic"
)

// Job represents work to be done
type Job func()

// Pool manages a pool of goroutines for parallel I/O
type Pool struct {
	jobs    chan Job
	wg      sync.WaitGroup
	quit    chan struct{}
	active  int32
	started bool
}

// New creates a new worker pool with the specified number of workers
func New(numWorkers int) *Pool {
	if numWorkers <= 0 {
		numWorkers = 4
	}

	p := &Pool{
		jobs: make(chan Job, 1000),
		quit: make(chan struct{}),
	}

	for i := 0; i < numWorkers; i++ {
		p.wg.Add(1)
		go p.worker()
	}

	p.started = true
	return p
}

func (p *Pool) worker() {
	defer p.wg.Done()
	for {
		select {
		case job, ok := <-p.jobs:
			if !ok {
				return
			}
			atomic.AddInt32(&p.active, 1)
			job()
			atomic.AddInt32(&p.active, -1)
		case <-p.quit:
			return
		}
	}
}

// Submit adds a job to the pool
func (p *Pool) Submit(job Job) {
	if job == nil {
		return
	}
	select {
	case p.jobs <- job:
	case <-p.quit:
	}
}

// Active returns the number of currently running jobs
func (p *Pool) Active() int32 {
	return atomic.LoadInt32(&p.active)
}

// Stop gracefully stops the pool
func (p *Pool) Stop() {
	if p.started {
		p.started = false
		close(p.quit)
		p.wg.Wait()
	}
}
