# seqpool
seqpool 是一個 支援多類型任務並行處理、但能確保 全域輸出順序一致 的 Golang worker pool。

適用於當你希望：

不同類型的任務可並行處理（類型之間並不影響）

同一類型的任務以順序處理

所有任務輸出時必須依照提交的順序顯示（全域序號）

# 特性 Features
✅ 支援 多類型任務 並行處理

✅ 同類型任務使用獨立 worker，保序執行

✅ 全域結果會依照 提交順序序號輸出

✅ 動態建立與回收 worker，根據型別與資源使用

✅ 可監控因 worker 資源不足而 等待中的新型別任務

# 安裝方式
```bash
go get github.com/akilakuma/seqpool
```

# 使用範例
```go
package main

import (
   "fmt"
   "math/rand"
   "time"

   "github.com/akilakuma/seqpool"
)

func main() {
   rand.Seed(time.Now().UnixNano())
   fn := func() {
      time.Sleep(time.Duration(rand.Intn(1000)) * time.Millisecond)
   }

   config := seqpool.Config{
      TaskBuffer:          100,
      WorkBuffer:          100,
      ResultBuffer:        100,
      MaxDedicatedWorkers: 3,
      WorkerIdleTimeout:   5 * time.Second,
   }
   tasksIn := seqpool.NewSeq(config)

   go func() {
      types := []string{"A", "B", "C", "D", "E"}
      for i := 0; i < 10; i++ {
         typ := types[rand.Intn(len(types))]
         t := seqpool.Task{
            Type: typ,
            Name: fmt.Sprintf("%s-task-%d", typ, i),
            Fn:   fn,
         }
         tasksIn <- t
         time.Sleep(500 * time.Millisecond)
      }
      close(tasksIn)
   }()

   go func() {
      for pending := range seqpool.NewTypePendingNotification() {
         fmt.Printf("[Monitor] Detected waiting new type task: %s (Type: %s)\n", pending.Name, pending.Type)
      }
   }()

   select {
   case <-time.After(30 * time.Second):
      fmt.Println("All tasks done.")
   }
}


```
# 設計理念
1. 類型分工（Type-Based Dedicated Workers）
   每種任務類型都會有一個專屬的 worker，在該類型下的任務會依序處理。

2. 並行限制（Semaphore）
   使用 MaxDedicatedWorkers 限制總共同時存在的 worker 數量，避免過度佔用資源。

3. 排隊與監控
   當 worker 數量達上限，新的型別任務會暫存，並透過 NewTypePendingNotification 提供即時等待狀態通知。

4. 全域順序輸出（Aggregator）
   所有任務無論類型，輸出順序都會依照傳入順序的 GlobalSeq 序號，確保使用者在結果處理上簡單一致。

# API 說明
func NewSeq(config Config) chan<- Task
建立並啟動整個任務處理流程，回傳可供外部送入任務的 channel。

### type Task struct
```go
type Task struct {
	GlobalSeq int
	Type      string
	Name      string
	Fn        func()
}
```

### type Config struct
```go
type Config struct {
	TaskBuffer          int
	WorkBuffer          int
	ResultBuffer        int
	MaxDedicatedWorkers int
	WorkerIdleTimeout   time.Duration
}

```
func NewTypePendingNotification() <-chan Task
提供可讀取的新型別等待任務 channel，讓外部可用於監控或調整資源。
這個通道會在：
- 任務類型首次出現
- 當下無法建立新 worker（因為 worker 已達上限）


### 流程圖
```go
NewSeq() 回傳 tasksIn
        │
        ▼
  外部送入 Task（tasksIn）
        │
        ▼
 dispatcher:
   ├─ 如果已有 worker：直接丟進 worker channel
   ├─ 如果無 worker：
   │    ├─ semaphore 可用：創建 worker，立刻送 task
   │    └─ semaphore 不可用：task 丟進 pendingNewTypes
   │                           並推進 pendingNewTypeChan
        │
        ▼
 dedicatedWorker（per Type）執行 task.Fn()
        │
        ▼
 resultChan 收到結果 → aggregator 負責按 GlobalSeq 順序輸出
```


# 授權 License
MIT License