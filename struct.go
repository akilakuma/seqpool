package seqpool

import "time"

// Task is a struct representing a task, containing global sequence number, task type and name
type Task struct {
	GlobalSeq int
	Type      string
	Name      string
	Fn        func()
	// Additional fields can be added as needed
}

// Config contains system configuration parameters
type Config struct {
	TaskBuffer          int           // Input task channel buffer size
	WorkBuffer          int           // Worker channel buffer size
	ResultBuffer        int           // Output result channel buffer size
	MaxDedicatedWorkers int           // Maximum number of concurrent dedicated workers
	WorkerIdleTimeout   time.Duration // Duration after which an idle worker exits
}
