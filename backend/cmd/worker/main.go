// Command worker runs the scheduler and queue consumers (funding watcher,
// auto-release, payout retry).
package main

import (
	"os"

	"github.com/harundarat/vecrow/backend/internal/bootstrap"
)

func main() {
	os.Exit(bootstrap.Main("vecrow-worker", bootstrap.NewWorker))
}
