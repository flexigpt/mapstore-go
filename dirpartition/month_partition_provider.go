package dirpartition

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/flexigpt/mapstore-go"
)

// TimeExtractor is a function that returns the creation time of a file.
type TimeExtractor func(key mapstore.FileKey) (time.Time, error)

// MonthPartitionProvider decides directories yyyyMM from TimeExtractor.
type MonthPartitionProvider struct {
	TimeFn TimeExtractor
}

// GetPartitionDir implements the PartitionProvider interface.
func (p *MonthPartitionProvider) GetPartitionDir(key mapstore.FileKey) (string, error) {
	t, err := p.TimeFn(key)
	if err != nil {
		return "", fmt.Errorf("could not get time for file: %s err: %w", key.FileName, err)
	}
	return t.Format("200601"), nil
}

// ListPartitions returns a paginated and sorted list of partition directories in the base directory.
func (p *MonthPartitionProvider) ListPartitions(
	baseDir string,
	sortOrder string,
	pageToken string,
	pageSize int,
) (partitions []string, nextPageToken string, err error) {
	return listDirs(baseDir, sortOrder, pageToken, pageSize)
}

// listDirs returns a paginated and sorted list of directories in the base directory.
func listDirs(
	baseDir string,
	sortOrder string,
	pageToken string,
	pageSize int,
) (dirs []string, nextPageToken string, err error) {
	entries, err := os.ReadDir(baseDir)
	if err != nil {
		return nil, "", fmt.Errorf("failed to read base directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			dirs = append(dirs, entry.Name())
		}
	}

	// Sort partitions.
	switch strings.ToLower(sortOrder) {
	case mapstore.SortOrderAscending:
		sort.Strings(dirs)
	case mapstore.SortOrderDescending:
		sort.Sort(sort.Reverse(sort.StringSlice(dirs)))
	default:
		return nil, "", fmt.Errorf("invalid sort order: %s", sortOrder)
	}

	// Decode page token.
	start := 0
	if pageToken != "" {
		tokenData, err := base64.StdEncoding.DecodeString(pageToken)
		if err != nil {
			return nil, "", fmt.Errorf("invalid page token: %w", err)
		}
		if err := json.Unmarshal(tokenData, &start); err != nil {
			return nil, "", fmt.Errorf("invalid page token: %w", err)
		}
	}

	// Apply pagination.
	end := min(start+pageSize, len(dirs))

	// Generate next page token.
	if end < len(dirs) {
		nextpageTokenData, _ := json.Marshal(end)
		nextPageToken = base64.StdEncoding.EncodeToString(nextpageTokenData)
	}

	return dirs[start:end], nextPageToken, nil
}
