package runtime

import (
	"os"
	"strconv"
	"sync"
	"time"
	"unsafe"
)

const (
	defaultBlockingWorkers = 4
	maxBlockingWorkers     = 256
	defaultMaxBlockingJobs = 1024
	hardMaxBlockingJobs    = 1_000_000
)

type nativeBlockingJob struct {
	token  uintptr
	entry  unsafe.Pointer
	state  unsafe.Pointer
	done   chan struct{}
	once   sync.Once
	mu     sync.Mutex
	waiter uintptr
}

var nativeBlocking = struct {
	sync.Mutex
	started   bool
	stopping  bool
	workers   int
	maxJobs   int
	queue     chan *nativeBlockingJob
	wg        sync.WaitGroup
	jobs      map[uintptr]*nativeBlockingJob
	active    uint64
	peak      uint64
	submitted uint64
	completed uint64
}{jobs: map[uintptr]*nativeBlockingJob{}}

func nativeBlockingBindScheduler(current, prepare, cancel, wake, help uintptr) {}

func parseBlockingLimit(name string, fallback, hardMax int) int {
	raw := os.Getenv(name)
	if raw == "" {
		return fallback
	}
	value, err := strconv.ParseUint(raw, 10, 64)
	if err != nil || value == 0 {
		return fallback
	}
	if value > uint64(hardMax) {
		return hardMax
	}
	return int(value)
}

func nativeBlockingWorker(queue <-chan *nativeBlockingJob) {
	defer nativeBlocking.wg.Done()
	for job := range queue {
		callNativeEntry1(uintptr(job.entry), job.state)
		completeNativeBlockingJob(job)
	}
}
func ensureNativeBlockingStarted() bool {
	nativeBlocking.Lock()
	defer nativeBlocking.Unlock()
	if nativeBlocking.started {
		return !nativeBlocking.stopping
	}
	workers := parseBlockingLimit("TSNATIVE_BLOCKING_WORKERS", defaultBlockingWorkers, maxBlockingWorkers)
	maxJobs := parseBlockingLimit("TSNATIVE_MAX_BLOCKING_JOBS", defaultMaxBlockingJobs, hardMaxBlockingJobs)
	nativeBlocking.workers = workers
	nativeBlocking.maxJobs = maxJobs
	nativeBlocking.queue = make(chan *nativeBlockingJob, maxJobs)
	nativeBlocking.started = true
	nativeBlocking.stopping = false
	nativeBlocking.active = 0
	nativeBlocking.peak = 0
	nativeBlocking.submitted = 0
	nativeBlocking.completed = 0
	for i := 0; i < workers; i++ {
		nativeBlocking.wg.Add(1)
		go nativeBlockingWorker(nativeBlocking.queue)
	}
	return true
}

func lookupNativeBlockingJob(raw unsafe.Pointer) *nativeBlockingJob {
	if raw == nil {
		return nil
	}
	nativeBlocking.Lock()
	job := nativeBlocking.jobs[uintptr(raw)]
	nativeBlocking.Unlock()
	return job
}

func completeNativeBlockingJob(job *nativeBlockingJob) {
	job.once.Do(func() {
		nativeBlocking.Lock()
		if nativeBlocking.active > 0 {
			nativeBlocking.active--
		}
		nativeBlocking.completed++
		nativeBlocking.Unlock()

		job.mu.Lock()
		waiter := job.waiter
		job.waiter = 0
		close(job.done)
		job.mu.Unlock()
		if waiter != 0 {
			_ = schedulerWakeTask(waiter)
		}
	})
}

func nativeBlockingSubmit(entry unsafe.Pointer, state unsafe.Pointer) unsafe.Pointer {
	if entry == nil || !ensureNativeBlockingStarted() {
		return nil
	}
	nativeBlocking.Lock()
	if nativeBlocking.stopping || nativeBlocking.active >= uint64(nativeBlocking.maxJobs) {
		nativeBlocking.Unlock()
		return nil
	}
	tokenPtr := allocNativeHandle()
	job := &nativeBlockingJob{
		token: uintptr(tokenPtr), entry: entry, state: state, done: make(chan struct{}),
	}
	nativeBlocking.jobs[job.token] = job
	nativeBlocking.active++
	nativeBlocking.submitted++
	if nativeBlocking.active > nativeBlocking.peak {
		nativeBlocking.peak = nativeBlocking.active
	}
	nativeBlocking.queue <- job
	nativeBlocking.Unlock()
	return tokenPtr
}

func nativeBlockingJobWaitTask(raw unsafe.Pointer) int {
	job := lookupNativeBlockingJob(raw)
	if job == nil {
		return -1
	}
	select {
	case <-job.done:
		return 1
	default:
	}
	task := schedulerCurrentTaskPtr()
	if task == 0 || schedulerPreparePark() != 0 {
		return -1
	}
	job.mu.Lock()
	select {
	case <-job.done:
		job.mu.Unlock()
		schedulerCancelPark()
		return 1
	default:
	}
	if job.waiter != 0 {
		job.mu.Unlock()
		schedulerCancelPark()
		return -1
	}
	job.waiter = task
	job.mu.Unlock()
	return 0
}

func nativeBlockingJobWaitCooperative(raw unsafe.Pointer) {
	job := lookupNativeBlockingJob(raw)
	if job == nil {
		nativeAbort("invalid blocking job")
	}
	for {
		select {
		case <-job.done:
			return
		default:
		}
		if schedulerHelpOnce() != 0 {
			continue
		}
		select {
		case <-job.done:
			return
		case <-time.After(100 * time.Microsecond):
		}
	}
}

func nativeBlockingJobRelease(raw unsafe.Pointer) {
	if raw == nil {
		return
	}
	nativeBlockingJobWaitCooperative(raw)
	token := uintptr(raw)
	nativeBlocking.Lock()
	delete(nativeBlocking.jobs, token)
	nativeBlocking.Unlock()
	freeNativeHandle(raw)
}

func nativeBlockingPoolShutdown() {
	nativeBlocking.Lock()
	if !nativeBlocking.started {
		nativeBlocking.Unlock()
		return
	}
	if nativeBlocking.stopping {
		nativeBlocking.Unlock()
		nativeBlocking.wg.Wait()
		return
	}
	nativeBlocking.stopping = true
	queue := nativeBlocking.queue
	close(queue)
	nativeBlocking.Unlock()
	nativeBlocking.wg.Wait()
	nativeBlocking.Lock()
	nativeBlocking.started = false
	nativeBlocking.stopping = false
	nativeBlocking.workers = 0
	nativeBlocking.maxJobs = 0
	nativeBlocking.queue = nil
	nativeBlocking.Unlock()
}

func nativeBlockingWorkerCount() uintptr {
	nativeBlocking.Lock()
	workers := nativeBlocking.workers
	started := nativeBlocking.started
	nativeBlocking.Unlock()
	if !started {
		workers = parseBlockingLimit("TSNATIVE_BLOCKING_WORKERS", defaultBlockingWorkers, maxBlockingWorkers)
	}
	return uintptr(workers)
}

func nativeBlockingActiveJobs() uintptr {
	nativeBlocking.Lock()
	value := nativeBlocking.active
	nativeBlocking.Unlock()
	return uintptr(value)
}

func nativeBlockingPeakActiveJobs() uintptr {
	nativeBlocking.Lock()
	value := nativeBlocking.peak
	nativeBlocking.Unlock()
	return uintptr(value)
}

func nativeBlockingSubmittedJobs() uint64 {
	nativeBlocking.Lock()
	value := nativeBlocking.submitted
	nativeBlocking.Unlock()
	return value
}

func nativeBlockingCompletedJobs() uint64 {
	nativeBlocking.Lock()
	value := nativeBlocking.completed
	nativeBlocking.Unlock()
	return value
}
