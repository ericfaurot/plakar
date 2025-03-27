package reporting

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"runtime"

	"github.com/PlakarKorp/plakar/cmd/plakar/utils"
	"github.com/PlakarKorp/plakar/logging"
	"github.com/PlakarKorp/plakar/snapshot/header"
)

type ReportSnapshot struct {
	Header *header.Header
}

type ReportTask struct {
	Type         string
	Name         string
	Command      string
	Duration     string
	Status       string
	ErrorCode    string
	ErrorMessage string
}

type Report struct {
	TimeStamp     string
	Type          string
	Task         *ReportTask
	Snapshot     *ReportSnapshot
}

type Reporter interface {
	Emit(report Report) error
}

type HTTPReporter struct {
	logger *logging.Logger
	url     string
	retry   uint8
}

func (reporter *HTTPReporter) Emit(report Report) {
	data, err := json.Marshal(report)
	if err != nil {
		reporter.logger.Error("failed to encode report: %s", err)
		return
	}
	for _ = range reporter.retry {
		err := reporter.tryEmit(data)
		if err == nil {
			return
		}
		reporter.logger.Warn("failed to emit report: %s", err)
	}
	reporter.logger.Error("failed to emit report after %d tries", reporter.retry)
}

func (reporter *HTTPReporter) tryEmit(data []byte) error {
	req, err := http.NewRequest("POST", reporter.url,  bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", fmt.Sprintf("plakar/%s (%s/%s)", utils.VERSION, runtime.GOOS, runtime.GOARCH))
	req.Header.Set("Content-Type", "application/json")

	client := http.Client{}
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if 200 <= res.StatusCode && res.StatusCode < 300 {
		return nil
	}

	return fmt.Errorf("request failed with status %s", res.Status)
}
