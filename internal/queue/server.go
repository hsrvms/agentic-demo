package queue

import (
	"time"

	"github.com/hibiken/asynq"
)

// Default queue weights determine worker allocation across queues.
var defaultQueues = map[string]int{
	QueueIngestion: 3,
	QueueReport:    2,
	QueueDelivery:  1,
}

// ServerConfig configures the asynq worker server.
type ServerConfig struct {
	RedisAddr   string
	Concurrency int
	Queues      map[string]int
	MaxRetry    int
}

// WorkerServer wraps asynq.Server with lifecycle management.
type WorkerServer struct {
	server *asynq.Server
	mux    *asynq.ServeMux
}

// NewWorkerServer creates a WorkerServer with the given config and handler deps.
// If cfg.Queues is nil, default queue weights are used.
func NewWorkerServer(cfg ServerConfig, deps HandlerDeps) *WorkerServer {
	queues := cfg.Queues
	if queues == nil {
		queues = defaultQueues
	}

	concurrency := cfg.Concurrency
	if concurrency <= 0 {
		concurrency = 10
	}

	srv := asynq.NewServer(
		asynq.RedisClientOpt{Addr: cfg.RedisAddr},
		asynq.Config{
			Concurrency: concurrency,
			Queues:      queues,
			RetryDelayFunc: func(n int, err error, t *asynq.Task) time.Duration {
				return time.Duration(n) * 30 * time.Second
			},
		},
	)

	mux := asynq.NewServeMux()
	RegisterHandlers(mux, deps)

	return &WorkerServer{server: srv, mux: mux}
}

// Start begins processing tasks. It is non-blocking.
func (s *WorkerServer) Start() error {
	return s.server.Start(s.mux)
}

// Stop initiates graceful shutdown, waiting for in-flight tasks to complete.
func (s *WorkerServer) Stop() {
	s.server.Shutdown()
}
