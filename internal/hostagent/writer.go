package hostagent

import (
	"io"
	"os"
	"sync"
)

// syncWriter serializes JSON event writes and makes regular log files durable.
type syncWriter struct {
	mutex  sync.Mutex
	output io.Writer
}

// Write serializes one event and syncs a regular destination file.
func (writer *syncWriter) Write(data []byte) (int, error) {
	writer.mutex.Lock()
	defer writer.mutex.Unlock()

	count, err := writer.output.Write(data)
	if err != nil {
		return count, err
	}

	file, ok := writer.output.(*os.File)
	if !ok {
		return count, nil
	}

	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return count, nil
	}

	return count, file.Sync()
}
