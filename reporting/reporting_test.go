package reporting

import (
	"os"
	"testing"

	"github.com/PlakarKorp/plakar/logging"
)

func TestEmit(t *testing.T) {

	logger := logging.NewLogger(os.Stdout, os.Stderr)
	
	reporter := HTTPReporter{
		logger: logger,
		url: "http://localhost:8080/report",
		retry: 3,
	}

	report := Report{
		Task: &ReportTask{
			Type:         "test",
			Name:         "test",
			Command:      "ls",
			Duration:     "0",
			Status:       "OK",
			ErrorCode:    "",
			ErrorMessage: "",
		},
	}

	reporter.Emit(report)
}
