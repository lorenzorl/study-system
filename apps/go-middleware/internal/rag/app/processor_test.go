package app_test

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/app"
)

// fakeDocumentProcess is a hand-written DocumentProcess. It records what the
// processor ran, can fail chosen documents, and can measure how many calls ran
// at once, so the processor is exercised with no PDF, database, or network.
type fakeDocumentProcess struct {
	mu       sync.Mutex
	ran      []string
	failures map[string]error

	active    int
	maxActive int
	delay     time.Duration
}

func (f *fakeDocumentProcess) Execute(ctx context.Context, documentID string) (app.ProcessReport, error) {
	f.mu.Lock()
	f.active++
	if f.active > f.maxActive {
		f.maxActive = f.active
	}
	f.mu.Unlock()

	if f.delay > 0 {
		time.Sleep(f.delay)
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	f.active--
	f.ran = append(f.ran, documentID)
	if err, ok := f.failures[documentID]; ok {
		return app.ProcessReport{DocumentID: documentID}, err
	}
	return app.ProcessReport{DocumentID: documentID}, nil
}

func (f *fakeDocumentProcess) processed() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.ran...)
}

func (f *fakeDocumentProcess) peak() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.maxActive
}

// flakyDocumentProcess fails its first call and succeeds afterwards, so a retry
// of the same document is observable.
type flakyDocumentProcess struct {
	mu    sync.Mutex
	calls int
}

func (f *flakyDocumentProcess) Execute(_ context.Context, documentID string) (app.ProcessReport, error) {
	f.mu.Lock()
	f.calls++
	call := f.calls
	f.mu.Unlock()

	if call == 1 {
		return app.ProcessReport{DocumentID: documentID}, errors.New("the model is down")
	}
	return app.ProcessReport{DocumentID: documentID}, nil
}

func TestBackgroundProcessor_ProcessesEverySubmittedDocument(t *testing.T) {
	process := &fakeDocumentProcess{failures: map[string]error{}}
	processor := app.NewBackgroundProcessor(t.Context(), 2, process)

	for _, id := range []string{"a", "b", "c", "d", "e"} {
		processor.Submit(id)
	}
	processor.Wait()

	assert.ElementsMatch(t, []string{"a", "b", "c", "d", "e"}, process.processed())
}

func TestBackgroundProcessor_BoundsConcurrencyToTheWorkerCount(t *testing.T) {
	process := &fakeDocumentProcess{failures: map[string]error{}, delay: 20 * time.Millisecond}
	processor := app.NewBackgroundProcessor(t.Context(), 2, process)

	for i := 0; i < 8; i++ {
		processor.Submit(strconv.Itoa(i))
	}
	processor.Wait()

	assert.LessOrEqual(t, process.peak(), 2, "the pool must never run more than its workers")
	assert.Len(t, process.processed(), 8)
}

func TestBackgroundProcessor_RecordsTheLastFailure(t *testing.T) {
	process := &fakeDocumentProcess{failures: map[string]error{"bad": errors.New("the model is down")}}
	processor := app.NewBackgroundProcessor(t.Context(), 1, process)

	processor.Submit("bad")
	processor.Wait()

	message, ok := processor.LastFailure("bad")
	require.True(t, ok)
	assert.Contains(t, message, "the model is down")

	_, ok = processor.LastFailure("good")
	assert.False(t, ok)
}

func TestBackgroundProcessor_ASuccessfulRetryClearsTheFailure(t *testing.T) {
	processor := app.NewBackgroundProcessor(t.Context(), 1, &flakyDocumentProcess{})

	// The same document is submitted twice: the first attempt fails, the second
	// succeeds, and the last failure must not survive a success.
	processor.Submit("doc")
	processor.Submit("doc")
	processor.Wait()

	_, ok := processor.LastFailure("doc")
	assert.False(t, ok, "a successful retry clears the recorded failure")
}
