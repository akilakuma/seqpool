package seqpool

import (
	"fmt"
	"time"
)

// pendingNewTypeChan notifies external systems when new type tasks are queued due to worker count reaching the limit
var pendingNewTypeChan = make(chan Task, 100)

// NewSeq initializes the entire processing workflow, returns tasksIn channel for external task submission
func NewSeq(config Config) chan<- Task {
	tasksIn := make(chan Task, config.TaskBuffer)
	resultChan := make(chan Task, config.ResultBuffer)
	// Create semaphore to limit the number of concurrent dedicated workers
	workerSemaphore := make(chan struct{}, config.MaxDedicatedWorkers)

	go aggregator(resultChan)
	go dispatcher(tasksIn, resultChan, workerSemaphore, config)
	return tasksIn
}

// aggregator is responsible for outputting results according to global sequence
func aggregator(resultChan <-chan Task) {
	nextSeq := 0
	// Buffer for storing received but non-consecutive results
	resultBuffer := make(map[int]Task)
	for {
		task, ok := <-resultChan
		if !ok {
			return
		}
		resultBuffer[task.GlobalSeq] = task
		// Output consecutively if the next result exists
		for {
			if t, exists := resultBuffer[nextSeq]; exists {
				fmt.Printf("[Aggregator] Output result: GlobalSeq=%d, Type=%s, Name=%s\n", t.GlobalSeq, t.Type, t.Name)
				delete(resultBuffer, nextSeq)
				nextSeq++
			} else {
				break
			}
		}
	}
}

// dispatcher receives tasks from tasksIn, assigns global sequence numbers, and dispatches to corresponding dedicated workers.
// If no worker exists for the type, it attempts to acquire a token non-blockingly to create a worker; if no token is available,
// the task is placed in the pendingNewTypes queue.
func dispatcher(tasksIn <-chan Task, resultChan chan<- Task, workerSemaphore chan struct{}, config Config) {
	// workerMap stores dedicated worker task channels for each type
	workerMap := make(map[string]chan Task)
	// pendingNewTypes stores new type tasks (key is task.Type, value is list of waiting tasks for that type)
	pendingNewTypes := make(map[string][]Task)
	seq := 0

	// Periodically check pendingNewTypes, create dedicated worker if token is available
	go func() {
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for range ticker.C {
			for typ, taskList := range pendingNewTypes {
				if len(taskList) > 0 {
					// Try to acquire token non-blockingly
					select {
					case workerSemaphore <- struct{}{}:
						// Successfully acquired token, create dedicated worker
						workerCh := make(chan Task, config.WorkBuffer)
						workerMap[typ] = workerCh
						go dedicatedWorker(typ, workerCh, resultChan, workerSemaphore, config.WorkerIdleTimeout)
						// Send all waiting tasks to worker channel
						for _, t := range taskList {
							workerCh <- t
						}
						// Clear tasks of this type from waiting queue
						delete(pendingNewTypes, typ)
						fmt.Printf("[Dispatcher] Created worker for waiting new type %s, processing %d waiting tasks\n", typ, len(taskList))
					default:
						// No token available, keep waiting
					}
				}
			}
		}
	}()

	// Main loop: continuously receive tasks from tasksIn
	for task := range tasksIn {
		// Assign global sequence number
		task.GlobalSeq = seq
		seq++
		if task.Name == "" {
			task.Name = fmt.Sprintf("%s%d", task.Type, task.GlobalSeq)
		}

		// If dedicated worker exists for this type, send task directly
		if workerCh, exists := workerMap[task.Type]; exists {
			workerCh <- task
		} else {
			// New type task, try to acquire token non-blockingly
			select {
			case workerSemaphore <- struct{}{}:
				// Token available, create dedicated worker
				workerCh := make(chan Task, config.WorkBuffer)
				workerMap[task.Type] = workerCh
				go dedicatedWorker(task.Type, workerCh, resultChan, workerSemaphore, config.WorkerIdleTimeout)
				workerCh <- task
				fmt.Printf("[Dispatcher] Created worker for new type %s\n", task.Type)
			default:
				// No token available, add task to waiting queue
				pendingNewTypes[task.Type] = append(pendingNewTypes[task.Type], task)
				// Notify external monitoring of waiting status
				pendingNewTypeChan <- task
				fmt.Printf("[Dispatcher] New type %s task %s entered waiting queue (worker count at limit)\n", task.Type, task.Name)
			}
		}
	}
}

// dedicatedWorker processes tasks of a specific type.
// It uses an idle timer and exits if no tasks are received for idleTimeout duration,
// releasing the semaphore token before exit
func dedicatedWorker(taskType string,
	tasks <-chan Task,
	resultChan chan<- Task,
	workerSemaphore chan struct{},
	idleTimeout time.Duration,
) {
	fmt.Printf("[Worker %s] Started\n", taskType)
	timer := time.NewTimer(idleTimeout)
	defer func() {
		// Release semaphore token before worker ends
		<-workerSemaphore
		fmt.Printf("[Worker %s] Ended, token released\n", taskType)
	}()

	for {
		select {
		case task, ok := <-tasks:
			if !ok {
				fmt.Printf("[Worker %s] Task channel closed\n", taskType)
				return
			}
			// Reset idle timer when task received
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(idleTimeout)

			fmt.Printf("[Worker %s] Processing task %s (GlobalSeq: %d)\n", taskType, task.Name, task.GlobalSeq)
			task.Fn()
			fmt.Printf("[Worker %s] Completed task %s (GlobalSeq: %d)\n", taskType, task.Name, task.GlobalSeq)
			resultChan <- task
		case <-timer.C:
			fmt.Printf("[Worker %s] Idle timeout (%v), exiting\n", taskType, idleTimeout)
			return
		}
	}
}

// NewTypePendingNotification returns a read-only channel for monitoring
// tasks that are waiting due to worker limit reached.
func NewTypePendingNotification() <-chan Task {
	return pendingNewTypeChan
}
