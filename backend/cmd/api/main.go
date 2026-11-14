// Command api serves the Vecrow REST API.
package main

import (
	"os"

	"github.com/harundarat/vecrow/backend/internal/bootstrap"
)

func main() {
	os.Exit(bootstrap.Main("vecrow-api", bootstrap.NewAPI))
}
