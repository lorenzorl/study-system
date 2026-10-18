package app

import (
	"context"
	"sync"
)

// DefaultProcessWorkers is how many documents the background processor runs at
// once. It is a constant, not a knob: the pipeline is CPU- and network-bound and
// this is a single-user local tool, so a small fixed pool is the right amount of
// concurrency and one fewer thing to configure.
const DefaultProcessWorkers = 2

// DocumentProcess runs the full pipeline for one document. It is the seam the
// background processor depends on, so the processor can be tested without a PDF,
// a database, or a model.
type DocumentProcess interface {
	Execute(ctx context.Context, documentID string) (ProcessReport, error)
}

// BackgroundProcessor runs documents on a bounded pool of workers.
//
// The queue is in memory and unbounded; only the workers are bounded. That is
// deliberate: a bounded queue would make Submit block the upload request, and a
// request that blocks is a request that fails (S2). The cost is that a server
// restart loses queued work, which the known limitation accepts: re-uploading
// the same bytes resumes the same document.
type BackgroundProcessor struct {
	process DocumentProcess
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup

	mu       sync.Mutex
	cond     *sync.Cond
	queue    []string
	closed   bool
	failures map[string]string
}

// NewBackgroundProcessor starts a processor with the given worker count and a
// context derived from parent, so a cancelled server context stops the workers.
// A worker count below 1 is treated as 1.
func NewBackgroundProcessor(parent context.Context, workers int, process DocumentProcess) *BackgroundProcessor {
	if workers < 1 {
		workers = 1
	}

	ctx, cancel := context.WithCancel(parent)
	processor := &BackgroundProcessor{
		process:  process,
		ctx:      ctx,
		cancel:   cancel,
		failures: map[string]string{},
	}
	processor.cond = sync.NewCond(&processor.mu)

	processor.wg.Add(workers)
	for i := 0; i < workers; i++ {
		go processor.worker()
	}

	return processor
}

// Submit queues a document for processing. It never blocks the caller and is a
// no-op once the processor is draining, so an upload racing shutdown cannot hang
// a request.
//
// A new attempt supersedes an earlier failure for the same document: the
// document is being tried again, and the status should say so rather than repeat
// the old error until the retry finishes.
func (p *BackgroundProcessor) Submit(documentID string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return
	}
	delete(p.failures, documentID)
	p.queue = append(p.queue, documentID)
	p.cond.Signal()
}

// LastFailure returns the message of the document's most recent failed run, if
// any. It is the processor's read side, which the status use case polls.
func (p *BackgroundProcessor) LastFailure(documentID string) (string, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()

	message, ok := p.failures[documentID]
	return message, ok
}

// Wait stops accepting work, drains the queue, and joins every worker. It is the
// graceful-shutdown drain the server calls.
func (p *BackgroundProcessor) Wait() {
	p.mu.Lock()
	p.closed = true
	p.cond.Broadcast()
	p.mu.Unlock()

	p.wg.Wait()
	p.cancel()
}

func (p *BackgroundProcessor) worker() {
	defer p.wg.Done()

	for {
		documentID, ok := p.next()
		if !ok {
			return
		}

		if _, err := p.process.Execute(p.ctx, documentID); err != nil {
			p.recordFailure(documentID, err)
			continue
		}
		p.clearFailure(documentID)
	}
}

// next returns the next queued document, blocking while the queue is empty and
// the processor is open. The second result is false once the processor is closed
// and the queue is empty.
func (p *BackgroundProcessor) next() (string, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()

	for len(p.queue) == 0 && !p.closed {
		p.cond.Wait()
	}
	if len(p.queue) == 0 {
		return "", false
	}

	documentID := p.queue[0]
	p.queue = p.queue[1:]
	return documentID, true
}

func (p *BackgroundProcessor) recordFailure(documentID string, err error) {
	// A cancelled context is the shutdown, not a document that failed. Recording
	// it would report every queued document as failed on the way out.
	if p.ctx.Err() != nil {
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	p.failures[documentID] = err.Error()
}

func (p *BackgroundProcessor) clearFailure(documentID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.failures, documentID)
}
