package seqpool

import (
	"fmt"
	"math/rand"
	"time"
)

// example: simulates sending tasks of multiple types while monitoring waiting new type tasks
func Example_basic() {

	rand.Seed(time.Now().UnixNano())
	fn := func() {
		time.Sleep(time.Duration(rand.Intn(1000)) * time.Millisecond)
	}

	config := Config{
		TaskBuffer:          100,
		WorkBuffer:          100,
		ResultBuffer:        100,
		MaxDedicatedWorkers: 3,               // Limit maximum concurrent dedicated workers to 3
		WorkerIdleTimeout:   5 * time.Second, // Worker exits after 5 seconds of idle time
	}
	tasksIn := NewSeq(config)

	// Simulate external submission of different type tasks
	go func() {
		types := []string{"A", "B", "C", "D", "E"}
		for i := 0; i < 10; i++ {
			// Randomly select a type
			typ := types[rand.Intn(len(types))]
			task := Task{
				Type: typ,
				Name: fmt.Sprintf("%s-task-%d", typ, i),
				Fn:   fn,
			}
			fmt.Printf("[Main] Sent task: %s\n", task.Name)
			tasksIn <- task
			time.Sleep(500 * time.Millisecond)
		}
		// Close tasksIn after all tasks are sent
		close(tasksIn)
	}()

	// Simulate external monitoring of waiting new type tasks
	go func() {
		for pendingTask := range NewTypePendingNotification() {
			fmt.Printf("[Monitor] Detected waiting new type task: %s (Type: %s)\n", pendingTask.Name, pendingTask.Type)
		}
	}()

	// Prevent main from ending too quickly
	select {
	case <-time.After(60 * time.Second):
		fmt.Println("Test completed")
	}
}
