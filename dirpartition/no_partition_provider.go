package dirpartition

import "github.com/flexigpt/mapstore-go"

// NoPartitionProvider is a default implementation that treats the base directory as a single partition.
type NoPartitionProvider struct{}

// GetPartitionDir returns an empty string, indicating no partitioning.
func (p *NoPartitionProvider) GetPartitionDir(_ mapstore.FileKey) (string, error) {
	return "", nil
}

// ListPartitions returns a single partition representing the base directory.
func (p *NoPartitionProvider) ListPartitions(
	baseDir string,
	sortOrder string,
	pageToken string,
	pageSize int,
) (partitions []string, nextPageToken string, err error) {
	return []string{""}, "", nil
}
