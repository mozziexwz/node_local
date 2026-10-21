package tasks

import (
	"encoding/json"
	"errors"
	"os"
)

type recordsFile struct {
	Version int      `json:"version"`
	Records []Record `json:"records"`
}

// loadRecords reads local\tasks.json. Records are a convenience: a missing
// file is empty, and a broken one is reported and replaced on the next write.
func loadRecords(path string) ([]Record, error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	var f recordsFile
	if err == nil {
		err = json.Unmarshal(raw, &f)
	}
	if err != nil {
		return nil, errors.New("任务记录文件 local\\tasks.json 无法读取，之前的记录不会显示")
	}
	if len(f.Records) > maxRecords {
		f.Records = f.Records[:maxRecords]
	}
	return f.Records, nil
}
