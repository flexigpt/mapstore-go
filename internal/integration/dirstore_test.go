package integration

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/flexigpt/mapstore-go"
	"github.com/flexigpt/mapstore-go/dirpartition"
	"github.com/flexigpt/mapstore-go/jsonencdec"
)

const (
	goosWindows = "windows"

	testKey   = "key"
	testValue = "value"

	testFileJSON  = "testfile.json"
	emptyFileJSON = "emptyfile.json"
	invalidDir    = "invalid"

	file1JSON = "file1.json"
	file2JSON = "file2.json"
	file3JSON = "file3.json"
	file4JSON = "file4.json"
	file5JSON = "file5.json"
	file6JSON = "file6.json"
	file7JSON = "file7.json"
	file8JSON = "file8.json"
	file9JSON = "file9.json"

	aJSON = "a.json"
	bJSON = "b.json"
	cJSON = "c.json"

	appleJSON       = "apple.json"
	apricotJSON     = "apricot.json"
	bananaJSON      = "banana.json"
	berryJSON       = "berry.json"
	cherryJSON      = "cherry.json"
	applePieJSON    = "apple_pie.json"
	bananaBreadJSON = "banana_bread.json"
	berryTartJSON   = "berry_tart.json"
	zebraJSON       = "zebra.json"

	partition202301 = "202301"
	partition202302 = "202302"
	partition202303 = "202303"
	partition202305 = "202305"
	partition202306 = "202306"
	partition202307 = "202307"
	partition202308 = "202308"
	partition202309 = "202309"

	ascendingTestName        = "Ascending"
	descendingTestName       = "Descending"
	invalidSortOrderValue    = "invalid"
	invalidSortOrderTestName = "InvalidSortOrder"

	applePrefix            = "apple"
	bananaPrefix           = "banana"
	berryPrefix            = "berry"
	zPrefix                = "z"
	notFoundPrefix         = "notfound"
	apPrefix               = "ap"
	bananaUnderscorePrefix = "banana_"
	appleUnderscorePrefix  = "apple_"
)

func TestMapDirectoryStore_CRUD(t *testing.T) {
	t.Parallel()

	now := time.Now()
	tests := []struct {
		name               string
		partitionProvider  mapstore.PartitionProvider
		filename           string
		data               map[string]any
		expectedPartition  string
		expectedFileExists bool
		expectError        bool
	}{
		{
			name:               "dirpartition.NoPartitionProvider - Create File",
			partitionProvider:  &dirpartition.NoPartitionProvider{},
			filename:           testFileJSON,
			data:               map[string]any{testKey: testValue},
			expectedPartition:  "",
			expectedFileExists: true,
			expectError:        false,
		},
		{
			name: "dirpartition.MonthPartitionProvider - Create File",
			partitionProvider: &dirpartition.MonthPartitionProvider{
				TimeFn: func(fileKey mapstore.FileKey) (time.Time, error) { return now, nil },
			},
			filename:           testFileJSON,
			data:               map[string]any{testKey: testValue},
			expectedPartition:  now.Format("200601"),
			expectedFileExists: true,
			expectError:        false,
		},
		{
			name:               "dirpartition.NoPartitionProvider - Empty Data",
			partitionProvider:  &dirpartition.NoPartitionProvider{},
			filename:           emptyFileJSON,
			data:               map[string]any{},
			expectedPartition:  "",
			expectedFileExists: true,
			expectError:        false,
		},
		{
			name:               "Invalid Directory (nested path without parent dir)",
			partitionProvider:  &dirpartition.NoPartitionProvider{},
			filename:           filepath.Join(invalidDir, testFileJSON),
			data:               map[string]any{testKey: testValue},
			expectedPartition:  "",
			expectedFileExists: false,
			expectError:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			baseDir := t.TempDir()
			mds, err := mapstore.NewMapDirectoryStore(
				baseDir,
				true,
				tt.partitionProvider,
				jsonencdec.JSONEncoderDecoder{},
			)
			if err != nil {
				t.Fatalf("failed to create MapDirectoryStore: %v", err)
			}

			err = mds.SetFileData(mapstore.FileKey{FileName: tt.filename}, tt.data)
			if tt.expectError {
				if err == nil {
					t.Fatalf("expected error but got none")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			partitionDir := filepath.Join(baseDir, tt.expectedPartition)
			filePath := filepath.Join(partitionDir, tt.filename)

			_, err = os.Stat(filePath)
			if tt.expectedFileExists {
				if os.IsNotExist(err) {
					t.Fatalf("expected file to exist but it does not")
				}
			} else {
				if !os.IsNotExist(err) {
					t.Fatalf("expected file not to exist but it does")
				}
			}

			if tt.expectedFileExists {
				data, err := mds.GetFileData(mapstore.FileKey{FileName: tt.filename}, false)
				if err != nil {
					t.Fatalf("failed to get file data: %v", err)
				}
				if len(data) != len(tt.data) {
					t.Fatalf("expected data length %d, got %d", len(tt.data), len(data))
				}
				for k, v := range tt.data {
					if data[k] != v {
						t.Fatalf("expected data[%s] = %v, got %v", k, v, data[k])
					}
				}
			}
		})
	}
}

func TestMapDirectoryStore_DeleteFile(t *testing.T) {
	t.Parallel()

	baseDir := t.TempDir()
	mds, err := mapstore.NewMapDirectoryStore(
		baseDir,
		true,
		&dirpartition.NoPartitionProvider{},
		jsonencdec.JSONEncoderDecoder{},
	)
	if err != nil {
		t.Fatalf("failed to create MapDirectoryStore: %v", err)
	}

	filename := testFileJSON
	err = mds.SetFileData(mapstore.FileKey{FileName: filename}, map[string]any{testKey: testValue})
	if err != nil {
		t.Fatalf("failed to set file data: %v", err)
	}

	filePath := filepath.Join(baseDir, filename)
	_, err = os.Stat(filePath)
	if os.IsNotExist(err) {
		t.Fatalf("expected file to exist but it does not")
	}

	err = mds.DeleteFile(mapstore.FileKey{FileName: filename})
	if err != nil {
		t.Fatalf("failed to delete file: %v", err)
	}

	_, err = os.Stat(filePath)
	if !os.IsNotExist(err) {
		t.Fatalf("expected file not to exist but it does")
	}
}

// Listing Tests: Basic, Pagination, Filtering, Prefix.

func TestMapDirectoryStore_ListFiles_BasicAndSort(t *testing.T) {
	baseDir := t.TempDir()

	fixedNow := time.Date(2025, 1, 15, 0, 0, 0, 0, time.UTC)
	partitionProvider := &dirpartition.MonthPartitionProvider{
		TimeFn: func(filekey mapstore.FileKey) (time.Time, error) { return fixedNow, nil },
	}
	mds, err := mapstore.NewMapDirectoryStore(
		baseDir,
		true,
		partitionProvider,
		jsonencdec.JSONEncoderDecoder{},
	)
	if err != nil {
		t.Fatalf("failed to create MapDirectoryStore: %v", err)
	}

	files := []string{file1JSON, file2JSON, file3JSON}
	for _, filename := range files {
		if err := mds.SetFileData(
			mapstore.FileKey{FileName: filename},
			map[string]any{testKey: testValue},
		); err != nil {
			t.Fatalf("failed to set file data: %v", err)
		}
	}

	tests := []struct {
		name          string
		sortOrder     string
		expectedFiles []string
		expectError   bool
	}{
		{
			name:          ascendingTestName,
			sortOrder:     mapstore.SortOrderAscending,
			expectedFiles: files,
		},
		{
			name:          descendingTestName,
			sortOrder:     mapstore.SortOrderDescending,
			expectedFiles: []string{file3JSON, file2JSON, file1JSON},
		},
		{
			name:        invalidSortOrderTestName,
			sortOrder:   invalidSortOrderValue,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			files, _, err := mds.ListFiles(mapstore.ListingConfig{SortOrder: tt.sortOrder}, "")
			if tt.expectError {
				if err == nil {
					t.Fatalf("expected error but got none")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			var filenames []string
			for _, f := range files {
				filenames = append(filenames, baseName(f.BaseRelativePath))
			}
			if len(filenames) != len(tt.expectedFiles) {
				t.Fatalf("expected %d files, got %d", len(tt.expectedFiles), len(filenames))
			}
			for i, expectedFile := range tt.expectedFiles {
				if filenames[i] != expectedFile {
					t.Fatalf("expected file %s, got %s", expectedFile, filenames[i])
				}
			}
		})
	}
}

func TestMapDirectoryStore_ListFiles_NoPartitionProvider_Pagination(t *testing.T) {
	baseDir := t.TempDir()
	files := []string{
		file1JSON, file2JSON, file3JSON, file4JSON, file5JSON,
		file6JSON, file7JSON, file8JSON, file9JSON,
	}
	testData := map[string]any{testKey: testValue}
	if err := os.MkdirAll(baseDir, os.ModePerm); err != nil {
		t.Fatalf("failed to create dir: %v", err)
	}
	for _, file := range files {
		filePath := filepath.Join(baseDir, file)
		fileData, err := json.Marshal(testData)
		if err != nil {
			t.Fatalf("failed to marshal test data: %v", err)
		}
		if err := os.WriteFile(filePath, fileData, 0o600); err != nil {
			t.Fatalf("failed to write test file: %v", err)
		}
	}

	tests := []struct {
		name          string
		sortOrder     string
		pageSize      int
		expectedPages [][]string
	}{
		{
			name:      ascendingTestName,
			sortOrder: mapstore.SortOrderAscending,
			pageSize:  4,
			expectedPages: [][]string{
				{file1JSON, file2JSON, file3JSON, file4JSON},
				{file5JSON, file6JSON, file7JSON, file8JSON},
				{file9JSON},
			},
		},
		{
			name:      descendingTestName,
			sortOrder: mapstore.SortOrderDescending,
			pageSize:  4,
			expectedPages: [][]string{
				{file9JSON, file8JSON, file7JSON, file6JSON},
				{file5JSON, file4JSON, file3JSON, file2JSON},
				{file1JSON},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pageToken := ""
			mds, err := mapstore.NewMapDirectoryStore(
				baseDir,
				true,
				&dirpartition.NoPartitionProvider{},
				jsonencdec.JSONEncoderDecoder{},
				mapstore.WithDirPageSize(tt.pageSize),
			)
			if err != nil {
				t.Fatalf("failed to create MapDirectoryStore: %v", err)
			}
			for pageIndex, expectedFiles := range tt.expectedPages {
				got, nextPageToken, err := mds.ListFiles(
					mapstore.ListingConfig{SortOrder: tt.sortOrder},
					pageToken,
				)
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if len(got) != len(expectedFiles) {
					t.Fatalf(
						"expected %d files on page %d, got %d",
						len(expectedFiles),
						pageIndex+1,
						len(got),
					)
				}
				for i, expectedFile := range expectedFiles {
					if normalizeRel(got[i].BaseRelativePath) != normalizeRel(expectedFile) {
						t.Fatalf(
							"expected file %q on page %d, got %q",
							expectedFile,
							pageIndex+1,
							got[i].BaseRelativePath,
						)
					}
				}
				pageToken = nextPageToken
			}
		})
	}
}

func TestMapDirectoryStore_ListFiles_MultiPartition_Pagination(t *testing.T) {
	baseDir := filepath.Join(t.TempDir(), "listdir")
	partitions := []string{partition202301, partition202302, partition202303}
	files := []string{file1JSON, file2JSON, file3JSON, file4JSON, file5JSON}
	testData := map[string]any{testKey: testValue}

	for _, partition := range partitions {
		partitionDir := filepath.Join(baseDir, partition)
		if err := os.MkdirAll(partitionDir, os.ModePerm); err != nil {
			t.Fatalf("failed to create partition directory: %v", err)
		}
		for _, file := range files {
			filePath := filepath.Join(partitionDir, file)
			fileData, err := json.Marshal(testData)
			if err != nil {
				t.Fatalf("failed to marshal test data: %v", err)
			}
			if err := os.WriteFile(filePath, fileData, 0o600); err != nil {
				t.Fatalf("failed to write test file: %v", err)
			}
		}
	}

	tests := []struct {
		name          string
		sortOrder     string
		pageSize      int
		expectedPages [][]string
	}{
		{
			name:      ascendingTestName,
			sortOrder: mapstore.SortOrderAscending,
			pageSize:  4,
			expectedPages: [][]string{
				{
					path.Join(partition202301, file1JSON),
					path.Join(partition202301, file2JSON),
					path.Join(partition202301, file3JSON),
					path.Join(partition202301, file4JSON),
				},
				{
					path.Join(partition202301, file5JSON),
					path.Join(partition202302, file1JSON),
					path.Join(partition202302, file2JSON),
					path.Join(partition202302, file3JSON),
				},
				{
					path.Join(partition202302, file4JSON),
					path.Join(partition202302, file5JSON),
					path.Join(partition202303, file1JSON),
					path.Join(partition202303, file2JSON),
				},
				{
					path.Join(partition202303, file3JSON),
					path.Join(partition202303, file4JSON),
					path.Join(partition202303, file5JSON),
				},
			},
		},
		{
			name:      descendingTestName,
			sortOrder: mapstore.SortOrderDescending,
			pageSize:  4,
			expectedPages: [][]string{
				{
					path.Join(partition202303, file5JSON),
					path.Join(partition202303, file4JSON),
					path.Join(partition202303, file3JSON),
					path.Join(partition202303, file2JSON),
				},
				{
					path.Join(partition202303, file1JSON),
					path.Join(partition202302, file5JSON),
					path.Join(partition202302, file4JSON),
					path.Join(partition202302, file3JSON),
				},
				{
					path.Join(partition202302, file2JSON),
					path.Join(partition202302, file1JSON),
					path.Join(partition202301, file5JSON),
					path.Join(partition202301, file4JSON),
				},
				{
					path.Join(partition202301, file3JSON),
					path.Join(partition202301, file2JSON),
					path.Join(partition202301, file1JSON),
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pageToken := ""
			for pageIndex, expectedFiles := range tt.expectedPages {
				fixedNow := time.Date(2025, 1, 15, 0, 0, 0, 0, time.UTC)
				partitionProvider := &dirpartition.MonthPartitionProvider{
					TimeFn: func(filekey mapstore.FileKey) (time.Time, error) { return fixedNow, nil },
				}
				mds, err := mapstore.NewMapDirectoryStore(
					baseDir,
					true,
					partitionProvider,
					jsonencdec.JSONEncoderDecoder{},
					mapstore.WithDirPageSize(tt.pageSize),
				)
				if err != nil {
					t.Fatalf("failed to create MapDirectoryStore: %v", err)
				}
				got, nextPageToken, err := mds.ListFiles(
					mapstore.ListingConfig{SortOrder: tt.sortOrder},
					pageToken,
				)
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if len(got) != len(expectedFiles) {
					t.Fatalf(
						"expected %d files on page %d, got %d",
						len(expectedFiles),
						pageIndex+1,
						len(got),
					)
				}
				for i, expectedFile := range expectedFiles {
					if normalizeRel(got[i].BaseRelativePath) != normalizeRel(expectedFile) {
						t.Fatalf(
							"expected file %q on page %d, got %q",
							expectedFile,
							pageIndex+1,
							got[i].BaseRelativePath,
						)
					}
				}
				pageToken = nextPageToken
			}
		})
	}
}

// Listing Tests: Filtering by Partition and FileName Prefix.

func TestMapDirectoryStore_ListFiles_FilteredPartitions(t *testing.T) {
	baseDir := t.TempDir()
	partitions := []string{partition202301, partition202302, partition202303}
	files := []string{aJSON, bJSON, cJSON}
	createFiles(t, baseDir, partitions, files)

	fixedNow := time.Date(2025, 1, 15, 0, 0, 0, 0, time.UTC)
	partitionProvider := &dirpartition.MonthPartitionProvider{
		TimeFn: func(filekey mapstore.FileKey) (time.Time, error) { return fixedNow, nil },
	}
	mds, err := mapstore.NewMapDirectoryStore(
		baseDir,
		true,
		partitionProvider,
		jsonencdec.JSONEncoderDecoder{},
		mapstore.WithDirPageSize(10),
	)
	if err != nil {
		t.Fatalf("failed to create MapDirectoryStore: %v", err)
	}

	tests := []struct {
		name             string
		sortOrder        string
		filterPartitions []string
		expectedFiles    []string
	}{
		{
			name:             "Non-filtered, ascending",
			sortOrder:        mapstore.SortOrderAscending,
			filterPartitions: nil,
			expectedFiles: []string{
				path.Join(partition202301, aJSON), path.Join(partition202301, bJSON), path.Join(partition202301, cJSON),
				path.Join(partition202302, aJSON), path.Join(partition202302, bJSON), path.Join(partition202302, cJSON),
				path.Join(partition202303, aJSON), path.Join(partition202303, bJSON), path.Join(partition202303, cJSON),
			},
		},
		{
			name:             "Non-filtered, descending",
			sortOrder:        mapstore.SortOrderDescending,
			filterPartitions: nil,
			expectedFiles: []string{
				path.Join(partition202303, cJSON), path.Join(partition202303, bJSON), path.Join(partition202303, aJSON),
				path.Join(partition202302, cJSON), path.Join(partition202302, bJSON), path.Join(partition202302, aJSON),
				path.Join(partition202301, cJSON), path.Join(partition202301, bJSON), path.Join(partition202301, aJSON),
			},
		},
		{
			name:             "Filtered, single partition",
			sortOrder:        mapstore.SortOrderAscending,
			filterPartitions: []string{partition202302},
			expectedFiles: []string{
				path.Join(partition202302, aJSON),
				path.Join(partition202302, bJSON),
				path.Join(partition202302, cJSON),
			},
		},
		{
			name:             "Filtered, multiple partitions, custom order",
			sortOrder:        mapstore.SortOrderAscending,
			filterPartitions: []string{partition202303, partition202301},
			expectedFiles: []string{
				path.Join(partition202303, aJSON), path.Join(partition202303, bJSON), path.Join(partition202303, cJSON),
				path.Join(partition202301, aJSON), path.Join(partition202301, bJSON), path.Join(partition202301, cJSON),
			},
		},
		{
			name:             "Filtered, multiple partitions, descending",
			sortOrder:        mapstore.SortOrderDescending,
			filterPartitions: []string{partition202302, partition202301},
			expectedFiles: []string{
				path.Join(partition202302, cJSON), path.Join(partition202302, bJSON), path.Join(partition202302, aJSON),
				path.Join(partition202301, cJSON), path.Join(partition202301, bJSON), path.Join(partition202301, aJSON),
			},
		},
		{
			name:             "Filtered, empty partition list",
			sortOrder:        mapstore.SortOrderAscending,
			filterPartitions: []string{},
			expectedFiles: []string{
				path.Join(partition202301, aJSON), path.Join(partition202301, bJSON), path.Join(partition202301, cJSON),
				path.Join(partition202302, aJSON), path.Join(partition202302, bJSON), path.Join(partition202302, cJSON),
				path.Join(partition202303, aJSON), path.Join(partition202303, bJSON), path.Join(partition202303, cJSON),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, nextPageToken, err := mds.ListFiles(
				mapstore.ListingConfig{SortOrder: tt.sortOrder, FilterPartitions: tt.filterPartitions},
				"",
			)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if nextPageToken != "" {
				t.Fatalf("expected no next page token, got %q", nextPageToken)
			}
			if len(got) != len(tt.expectedFiles) {
				t.Fatalf("expected %d files, got %d", len(tt.expectedFiles), len(got))
			}
			for i, want := range tt.expectedFiles {
				if normalizeRel(got[i].BaseRelativePath) != normalizeRel(want) {
					t.Errorf("at %d: want %q, got %q", i, want, got[i].BaseRelativePath)
				}
			}
		})
	}
}

func TestMapDirectoryStore_ListFiles_FilteredPartitions_Pagination(t *testing.T) {
	baseDir := t.TempDir()
	partitions := []string{partition202301, partition202302}
	files := []string{aJSON, bJSON, cJSON, "d.json"}
	createFiles(t, baseDir, partitions, files)

	fixedNow := time.Date(2025, 1, 15, 0, 0, 0, 0, time.UTC)
	partitionProvider := &dirpartition.MonthPartitionProvider{
		TimeFn: func(filekey mapstore.FileKey) (time.Time, error) { return fixedNow, nil },
	}
	pageSize := 3
	mds, err := mapstore.NewMapDirectoryStore(
		baseDir,
		true,
		partitionProvider,
		jsonencdec.JSONEncoderDecoder{},
		mapstore.WithDirPageSize(pageSize),
	)
	if err != nil {
		t.Fatalf("failed to create MapDirectoryStore: %v", err)
	}

	tests := []struct {
		name             string
		sortOrder        string
		filterPartitions []string
		expectedPages    [][]string
	}{
		{
			name:             "Filtered, paginated, asc",
			sortOrder:        mapstore.SortOrderAscending,
			filterPartitions: []string{partition202301, partition202302},
			expectedPages: [][]string{
				{
					path.Join(partition202301, aJSON),
					path.Join(partition202301, bJSON),
					path.Join(partition202301, cJSON),
				},
				{
					path.Join(partition202301, "d.json"),
					path.Join(partition202302, aJSON),
					path.Join(partition202302, bJSON),
				},
				{path.Join(partition202302, cJSON), path.Join(partition202302, "d.json")},
			},
		},
		{
			name:             "Filtered, paginated, desc",
			sortOrder:        mapstore.SortOrderDescending,
			filterPartitions: []string{partition202302, partition202301},
			expectedPages: [][]string{
				{
					path.Join(partition202302, "d.json"),
					path.Join(partition202302, cJSON),
					path.Join(partition202302, bJSON),
				},
				{
					path.Join(partition202302, aJSON),
					path.Join(partition202301, "d.json"),
					path.Join(partition202301, cJSON),
				},
				{path.Join(partition202301, bJSON), path.Join(partition202301, aJSON)},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pageToken := ""
			for pageIdx, wantFiles := range tt.expectedPages {
				got, nextPageToken, err := mds.ListFiles(
					mapstore.ListingConfig{SortOrder: tt.sortOrder, FilterPartitions: tt.filterPartitions},
					pageToken,
				)
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if len(got) != len(wantFiles) {
					t.Fatalf(
						"page %d: expected %d files, got %d",
						pageIdx+1,
						len(wantFiles),
						len(got),
					)
				}
				for i, want := range wantFiles {
					if normalizeRel(got[i].BaseRelativePath) != normalizeRel(want) {
						t.Errorf("page %d, file %d: want %q, got %q", pageIdx+1, i, want, got[i].BaseRelativePath)
					}
				}
				pageToken = nextPageToken
				if pageIdx < len(tt.expectedPages)-1 && pageToken == "" {
					t.Fatalf("expected next page token for page %d, got empty", pageIdx+1)
				}
				if pageIdx == len(tt.expectedPages)-1 && pageToken != "" {
					t.Fatalf("expected no next page token for last page, got %q", pageToken)
				}
			}
		})
	}
}

func TestMapDirectoryStore_ListFiles_FilenamePrefixFiltering(t *testing.T) {
	baseDir := t.TempDir()
	partitions := []string{partition202301, partition202302}
	files := []string{
		appleJSON, apricotJSON, bananaJSON, berryJSON, cherryJSON,
		applePieJSON, bananaBreadJSON, berryTartJSON, zebraJSON,
	}
	createFiles(t, baseDir, partitions, files)

	fixedNow := time.Date(2025, 1, 15, 0, 0, 0, 0, time.UTC)
	partitionProvider := &dirpartition.MonthPartitionProvider{
		TimeFn: func(filekey mapstore.FileKey) (time.Time, error) { return fixedNow, nil },
	}
	mds, err := mapstore.NewMapDirectoryStore(
		baseDir,
		true,
		partitionProvider,
		jsonencdec.JSONEncoderDecoder{},
		mapstore.WithDirPageSize(20),
	)
	if err != nil {
		t.Fatalf("failed to create MapDirectoryStore: %v", err)
	}

	type want struct{ files []string }
	tests := []struct {
		name             string
		sortOrder        string
		filterPartitions []string
		filenamePrefix   string
		want             want
	}{
		{
			name:           "No prefix, ascending",
			sortOrder:      mapstore.SortOrderAscending,
			filenamePrefix: "",
			want: want{files: []string{
				path.Join(partition202301, appleJSON),
				path.Join(partition202301, applePieJSON),
				path.Join(partition202301, apricotJSON),
				path.Join(partition202301, bananaJSON),
				path.Join(partition202301, bananaBreadJSON),
				path.Join(partition202301, berryJSON),
				path.Join(partition202301, berryTartJSON),
				path.Join(partition202301, cherryJSON),
				path.Join(partition202301, zebraJSON),
				path.Join(partition202302, appleJSON),
				path.Join(partition202302, applePieJSON),
				path.Join(partition202302, apricotJSON),
				path.Join(partition202302, bananaJSON),
				path.Join(partition202302, bananaBreadJSON),
				path.Join(partition202302, berryJSON),
				path.Join(partition202302, berryTartJSON),
				path.Join(partition202302, cherryJSON),
				path.Join(partition202302, zebraJSON),
			}},
		},
		{
			name:           "Prefix 'apple', ascending",
			sortOrder:      mapstore.SortOrderAscending,
			filenamePrefix: applePrefix,
			want: want{files: []string{
				path.Join(partition202301, appleJSON), path.Join(partition202301, applePieJSON),
				path.Join(partition202302, appleJSON), path.Join(partition202302, applePieJSON),
			}},
		},
		{
			name:           "Prefix 'banana', ascending",
			sortOrder:      mapstore.SortOrderAscending,
			filenamePrefix: bananaPrefix,
			want: want{files: []string{
				path.Join(partition202301, bananaJSON), path.Join(partition202301, bananaBreadJSON),
				path.Join(partition202302, bananaJSON), path.Join(partition202302, bananaBreadJSON),
			}},
		},
		{
			name:           "Prefix 'berry', descending",
			sortOrder:      mapstore.SortOrderDescending,
			filenamePrefix: berryPrefix,
			want: want{files: []string{
				path.Join(partition202302, berryTartJSON), path.Join(partition202302, berryJSON),
				path.Join(partition202301, berryTartJSON), path.Join(partition202301, berryJSON),
			}},
		},
		{
			name:           "Prefix 'z', ascending",
			sortOrder:      mapstore.SortOrderAscending,
			filenamePrefix: zPrefix,
			want: want{files: []string{
				path.Join(partition202301, zebraJSON), path.Join(partition202302, zebraJSON),
			}},
		},
		{
			name:           "Prefix 'notfound', ascending",
			sortOrder:      mapstore.SortOrderAscending,
			filenamePrefix: notFoundPrefix,
			want:           want{files: []string{}},
		},
		{
			name:             "Prefix '', filtered partition",
			sortOrder:        mapstore.SortOrderAscending,
			filterPartitions: []string{partition202301},
			filenamePrefix:   "",
			want: want{files: []string{
				path.Join(partition202301, appleJSON),
				path.Join(partition202301, applePieJSON),
				path.Join(partition202301, apricotJSON),
				path.Join(partition202301, bananaJSON),
				path.Join(partition202301, bananaBreadJSON),
				path.Join(partition202301, berryJSON),
				path.Join(partition202301, berryTartJSON),
				path.Join(partition202301, cherryJSON),
				path.Join(partition202301, zebraJSON),
			}},
		},
		{
			name:             "Prefix 'ap', filtered partition",
			sortOrder:        mapstore.SortOrderAscending,
			filterPartitions: []string{partition202302},
			filenamePrefix:   apPrefix,
			want: want{files: []string{
				path.Join(
					partition202302,
					appleJSON,
				),
				path.Join(partition202302, applePieJSON),
				path.Join(partition202302, apricotJSON),
			}},
		},
		{
			name:             "Prefix 'berry', filtered partition, descending",
			sortOrder:        mapstore.SortOrderDescending,
			filterPartitions: []string{partition202301},
			filenamePrefix:   berryPrefix,
			want: want{files: []string{
				path.Join(partition202301, berryTartJSON), path.Join(partition202301, berryJSON),
			}},
		},
		{
			name:           "Prefix with underscore",
			sortOrder:      mapstore.SortOrderAscending,
			filenamePrefix: bananaUnderscorePrefix,
			want: want{files: []string{
				path.Join(partition202301, bananaBreadJSON), path.Join(partition202302, bananaBreadJSON),
			}},
		},
		{
			name:           "Prefix with special char",
			sortOrder:      mapstore.SortOrderAscending,
			filenamePrefix: appleUnderscorePrefix,
			want: want{files: []string{
				path.Join(partition202301, applePieJSON), path.Join(partition202302, applePieJSON),
			}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, nextPageToken, err := mds.ListFiles(
				mapstore.ListingConfig{
					SortOrder:        tt.sortOrder,
					FilterPartitions: tt.filterPartitions,
					FilenamePrefix:   tt.filenamePrefix,
				},
				"",
			)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if nextPageToken != "" {
				t.Fatalf("expected no next page token, got %q", nextPageToken)
			}
			if len(got) != len(tt.want.files) {
				t.Fatalf("expected %d files, got %d: %v", len(tt.want.files), len(got), got)
			}
			for i, want := range tt.want.files {
				if normalizeRel(got[i].BaseRelativePath) != normalizeRel(want) {
					t.Errorf("at %d: want %q, got %q", i, want, got[i].BaseRelativePath)
				}
			}
		})
	}
}

func TestMapDirectoryStore_ListFiles_FilenamePrefixFiltering_Pagination(t *testing.T) {
	baseDir := t.TempDir()
	partitions := []string{partition202301}
	files := []string{
		appleJSON, apricotJSON, bananaJSON, berryJSON, cherryJSON,
		applePieJSON, bananaBreadJSON, berryTartJSON, zebraJSON,
	}
	createFiles(t, baseDir, partitions, files)

	fixedNow := time.Date(2025, 1, 15, 0, 0, 0, 0, time.UTC)
	partitionProvider := &dirpartition.MonthPartitionProvider{
		TimeFn: func(filekey mapstore.FileKey) (time.Time, error) { return fixedNow, nil },
	}
	pageSize := 2
	mds, err := mapstore.NewMapDirectoryStore(
		baseDir,
		true,
		partitionProvider,
		jsonencdec.JSONEncoderDecoder{},
		mapstore.WithDirPageSize(pageSize),
	)
	if err != nil {
		t.Fatalf("failed to create MapDirectoryStore: %v", err)
	}

	type pageWant struct{ files []string }
	tests := []struct {
		name           string
		sortOrder      string
		filenamePrefix string
		expectedPages  []pageWant
	}{
		{
			name:           "Prefix 'apple', ascending, paginated",
			sortOrder:      mapstore.SortOrderAscending,
			filenamePrefix: applePrefix,
			expectedPages: []pageWant{
				{files: []string{path.Join(partition202301, appleJSON), path.Join(partition202301, applePieJSON)}},
			},
		},
		{
			name:           "Prefix 'b', ascending, paginated",
			sortOrder:      mapstore.SortOrderAscending,
			filenamePrefix: "b",
			expectedPages: []pageWant{
				{files: []string{path.Join(partition202301, bananaJSON), path.Join(partition202301, bananaBreadJSON)}},
				{files: []string{path.Join(partition202301, berryJSON), path.Join(partition202301, berryTartJSON)}},
			},
		},
		{
			name:           "Prefix 'berry', ascending, paginated",
			sortOrder:      mapstore.SortOrderAscending,
			filenamePrefix: berryPrefix,
			expectedPages: []pageWant{
				{files: []string{path.Join(partition202301, berryJSON), path.Join(partition202301, berryTartJSON)}},
			},
		},
		{
			name:           "Prefix 'z', ascending, paginated",
			sortOrder:      mapstore.SortOrderAscending,
			filenamePrefix: zPrefix,
			expectedPages: []pageWant{
				{files: []string{path.Join(partition202301, zebraJSON)}},
			},
		},
		{
			name:           "Prefix '', ascending, paginated",
			sortOrder:      mapstore.SortOrderAscending,
			filenamePrefix: "",
			expectedPages: []pageWant{
				{files: []string{path.Join(partition202301, appleJSON), path.Join(partition202301, applePieJSON)}},
				{files: []string{path.Join(partition202301, apricotJSON), path.Join(partition202301, bananaJSON)}},
				{files: []string{path.Join(partition202301, bananaBreadJSON), path.Join(partition202301, berryJSON)}},
				{files: []string{path.Join(partition202301, berryTartJSON), path.Join(partition202301, cherryJSON)}},
				{files: []string{path.Join(partition202301, zebraJSON)}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pageToken := ""
			for pageIdx, want := range tt.expectedPages {
				got, nextPageToken, err := mds.ListFiles(
					mapstore.ListingConfig{
						SortOrder:      tt.sortOrder,
						FilenamePrefix: tt.filenamePrefix,
					},
					pageToken,
				)
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if len(got) != len(want.files) {
					t.Fatalf(
						"page %d: expected %d files, got %d: %v",
						pageIdx+1,
						len(want.files),
						len(got),
						got,
					)
				}
				for i, wantFile := range want.files {
					if normalizeRel(got[i].BaseRelativePath) != normalizeRel(wantFile) {
						t.Errorf(
							"page %d, file %d: want %q, got %q",
							pageIdx+1,
							i,
							wantFile,
							got[i].BaseRelativePath,
						)
					}
				}
				pageToken = nextPageToken
				if pageIdx < len(tt.expectedPages)-1 && pageToken == "" {
					t.Fatalf("expected next page token for page %d, got empty", pageIdx+1)
				}
				if pageIdx == len(tt.expectedPages)-1 && pageToken != "" {
					t.Fatalf("expected no next page token for last page, got %q", pageToken)
				}
			}
		})
	}
}

// Listing Tests: Partition Listing & Pagination.

func TestMapDirectoryStore_ListPartitions_Pagination(t *testing.T) {
	baseDir := t.TempDir()

	fixedNow := time.Date(2025, 1, 15, 0, 0, 0, 0, time.UTC)
	partitionProvider := &dirpartition.MonthPartitionProvider{
		TimeFn: func(filekey mapstore.FileKey) (time.Time, error) { return fixedNow, nil },
	}
	mds, err := mapstore.NewMapDirectoryStore(
		baseDir,
		true,
		partitionProvider,
		jsonencdec.JSONEncoderDecoder{},
	)
	if err != nil {
		t.Fatalf("failed to create MapDirectoryStore: %v", err)
	}

	partitions := []string{partition202301, partition202302, partition202303}
	for _, partition := range partitions {
		if err := os.Mkdir(filepath.Join(baseDir, partition), os.ModePerm); err != nil {
			t.Fatalf("failed to create partition directory: %v", err)
		}
	}

	tests := []struct {
		name          string
		sortOrder     string
		pageToken     string
		pageSize      int
		expectedParts []string
		expectError   bool
	}{
		{
			name:          ascendingTestName,
			sortOrder:     mapstore.SortOrderAscending,
			pageToken:     "",
			pageSize:      2,
			expectedParts: []string{partition202301, partition202302},
		},
		{
			name:          descendingTestName,
			sortOrder:     mapstore.SortOrderDescending,
			pageToken:     "",
			pageSize:      2,
			expectedParts: []string{partition202303, partition202302},
		},
		{
			name:        invalidSortOrderTestName,
			sortOrder:   invalidSortOrderValue,
			pageToken:   "",
			pageSize:    2,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, nextPageToken, err := mds.ListPartitions(
				baseDir, tt.sortOrder, tt.pageToken, tt.pageSize,
			)
			if tt.expectError {
				if err == nil {
					t.Fatalf("expected error but got none")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != len(tt.expectedParts) {
				t.Fatalf("expected %d partitions, got %d", len(tt.expectedParts), len(got))
			}
			for i, expectedPart := range tt.expectedParts {
				if got[i] != expectedPart {
					t.Fatalf("expected partition %s, got %s", expectedPart, got[i])
				}
			}
			if nextPageToken != "" {
				got2, _, err := mds.ListPartitions(
					baseDir, tt.sortOrder, nextPageToken, tt.pageSize,
				)
				if err != nil {
					t.Fatalf("unexpected error on next page: %v", err)
				}
				if len(got2) != 1 {
					t.Fatalf("expected 1 partition on next page, got %d", len(got2))
				}
			}
		})
	}
}

// Listing Tests: Edge Cases & Error Handling.

func TestMapDirectoryStore_ListFiles_ErrorsAndEdgeCases(t *testing.T) {
	t.Parallel()

	fixedNow := time.Date(2025, 1, 15, 0, 0, 0, 0, time.UTC)
	partitionProvider := &dirpartition.MonthPartitionProvider{
		TimeFn: func(filekey mapstore.FileKey) (time.Time, error) { return fixedNow, nil },
	}

	t.Run(invalidSortOrderTestName, func(t *testing.T) {
		t.Parallel()
		baseDir := t.TempDir()
		mds, err := mapstore.NewMapDirectoryStore(
			baseDir,
			true,
			partitionProvider,
			jsonencdec.JSONEncoderDecoder{},
		)
		if err != nil {
			t.Fatalf("failed to create MapDirectoryStore: %v", err)
		}
		_, _, err = mds.ListFiles(mapstore.ListingConfig{SortOrder: "notasort"}, "")
		if err == nil {
			t.Fatal("expected error for invalid sort order, got nil")
		}
	})

	t.Run("NonExistentPartition", func(t *testing.T) {
		t.Parallel()
		baseDir := t.TempDir()
		mds, err := mapstore.NewMapDirectoryStore(
			baseDir,
			true,
			partitionProvider,
			jsonencdec.JSONEncoderDecoder{},
		)
		if err != nil {
			t.Fatalf("failed to create MapDirectoryStore: %v", err)
		}
		got, nextPageToken, err := mds.ListFiles(
			mapstore.ListingConfig{
				SortOrder:        mapstore.SortOrderAscending,
				FilterPartitions: []string{"doesnotexist"},
			},
			"",
		)
		if err != nil {
			t.Fatalf("expected partition skipped for non-existent partition in filter, got err %s", err)
		}
		if len(got) != 0 {
			t.Fatalf("expected no files, got %v", got)
		}
		if nextPageToken != "" {
			t.Fatalf("expected no next page token, got %q", nextPageToken)
		}
	})

	t.Run("UnreadablePartitionDir", func(t *testing.T) {
		t.Parallel()

		// Windows ACLs don't behave like POSIX chmod; skip to keep the test portable.
		if runtime.GOOS == goosWindows {
			t.Skip("skipping chmod-based permission test on windows")
		}

		baseDir := t.TempDir()
		mds, err := mapstore.NewMapDirectoryStore(
			baseDir,
			true,
			partitionProvider,
			jsonencdec.JSONEncoderDecoder{},
		)
		if err != nil {
			t.Fatalf("failed to create MapDirectoryStore: %v", err)
		}

		partition := partition202301
		dir := filepath.Join(baseDir, partition)

		// Create dir & a file first, then remove permissions.
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("failed to create dir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(dir, aJSON), []byte(`{"k":"v"}`), 0o600); err != nil {
			t.Fatalf("failed to write file: %v", err)
		}

		if err := os.Chmod(dir, 0o000); err != nil {
			t.Skipf("chmod not supported/effective on this filesystem: %v", err)
		}
		defer func() { _ = os.Chmod(dir, 0o755) }()

		// Ensure it's actually unreadable; otherwise skip (e.g., elevated privileges).
		if _, err := os.ReadDir(dir); err == nil {
			t.Skip("directory is still readable after chmod 000 (likely elevated privileges); skipping")
		}

		got, nextPageToken, err := mds.ListFiles(
			mapstore.ListingConfig{
				SortOrder:        mapstore.SortOrderAscending,
				FilterPartitions: []string{partition},
			},
			"",
		)
		if err != nil {
			t.Fatalf("expected partition skipped for unreadable partition in filter, got err %s", err)
		}
		if len(got) != 0 {
			t.Fatalf("expected unreadable partition to be skipped (0 files), got %v", got)
		}
		if nextPageToken != "" {
			t.Fatalf("expected no next page token, got %q", nextPageToken)
		}
	})

	t.Run("InvalidPageToken", func(t *testing.T) {
		t.Parallel()
		baseDir := t.TempDir()
		mds, err := mapstore.NewMapDirectoryStore(
			baseDir,
			true,
			partitionProvider,
			jsonencdec.JSONEncoderDecoder{},
		)
		if err != nil {
			t.Fatalf("failed to create MapDirectoryStore: %v", err)
		}
		_, _, err = mds.ListFiles(mapstore.ListingConfig{SortOrder: mapstore.SortOrderAscending}, "notbase64!")
		if err == nil {
			t.Fatal("expected error for invalid base64 page token, got nil")
		}
		bad := base64.StdEncoding.EncodeToString([]byte("notjson"))
		_, _, err = mds.ListFiles(mapstore.ListingConfig{SortOrder: mapstore.SortOrderAscending}, bad)
		if err == nil {
			t.Fatal("expected error for invalid JSON page token, got nil")
		}
	})

	t.Run("CorruptedPageToken", func(t *testing.T) {
		t.Parallel()
		baseDir := t.TempDir()
		mds, err := mapstore.NewMapDirectoryStore(
			baseDir,
			true,
			partitionProvider,
			jsonencdec.JSONEncoderDecoder{},
		)
		if err != nil {
			t.Fatalf("failed to create MapDirectoryStore: %v", err)
		}
		bad := base64.StdEncoding.EncodeToString([]byte("{notjson:"))
		_, _, err = mds.ListFiles(mapstore.ListingConfig{SortOrder: mapstore.SortOrderAscending}, bad)
		if err == nil {
			t.Fatal("expected error for corrupted JSON page token, got nil")
		}
	})

	t.Run("EmptyBaseDir", func(t *testing.T) {
		t.Parallel()
		baseDir := t.TempDir()
		mds, err := mapstore.NewMapDirectoryStore(
			baseDir,
			true,
			partitionProvider,
			jsonencdec.JSONEncoderDecoder{},
		)
		if err != nil {
			t.Fatalf("failed to create MapDirectoryStore: %v", err)
		}
		got, nextPageToken, err := mds.ListFiles(mapstore.ListingConfig{SortOrder: mapstore.SortOrderAscending}, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 0 {
			t.Fatalf("expected no files, got %v", got)
		}
		if nextPageToken != "" {
			t.Fatalf("expected no next page token, got %q", nextPageToken)
		}
	})

	t.Run("FilterWithNonExistentPartition", func(t *testing.T) {
		t.Parallel()
		baseDir := t.TempDir()
		mds, err := mapstore.NewMapDirectoryStore(
			baseDir,
			true,
			partitionProvider,
			jsonencdec.JSONEncoderDecoder{},
		)
		if err != nil {
			t.Fatalf("failed to create MapDirectoryStore: %v", err)
		}
		partitions := []string{partition202302}
		files := []string{aJSON}
		createFiles(t, baseDir, partitions, files)
		_, _, err = mds.ListFiles(
			mapstore.ListingConfig{
				SortOrder:        mapstore.SortOrderAscending,
				FilterPartitions: []string{partition202301, "doesnotexist"},
			},
			"",
		)
		if err != nil {
			t.Fatalf("expected partition skipped for non-existent partition in filter, got err %s", err)
		}
	})

	t.Run("PageSizeLargerThanFiles", func(t *testing.T) {
		t.Parallel()
		baseDir := t.TempDir()
		partitions := []string{partition202303}
		files := []string{aJSON, bJSON}
		createFiles(t, baseDir, partitions, files)
		mds, err := mapstore.NewMapDirectoryStore(
			baseDir,
			true,
			partitionProvider,
			jsonencdec.JSONEncoderDecoder{},
			mapstore.WithDirPageSize(10),
		)
		if err != nil {
			t.Fatalf("failed to create MapDirectoryStore: %v", err)
		}
		got, nextPageToken, err := mds.ListFiles(mapstore.ListingConfig{SortOrder: mapstore.SortOrderAscending}, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 2 {
			t.Fatalf("expected 2 files, got %d", len(got))
		}
		if nextPageToken != "" {
			t.Fatalf("expected no next page token, got %q", nextPageToken)
		}
	})

	t.Run("EmptyFilterPartitions", func(t *testing.T) {
		t.Parallel()
		baseDir := t.TempDir()
		partitions := []string{partition202305}
		files := []string{aJSON}
		createFiles(t, baseDir, partitions, files)
		mds, err := mapstore.NewMapDirectoryStore(
			baseDir,
			true,
			partitionProvider,
			jsonencdec.JSONEncoderDecoder{},
		)
		if err != nil {
			t.Fatalf("failed to create MapDirectoryStore: %v", err)
		}
		got, nextPageToken, err := mds.ListFiles(
			mapstore.ListingConfig{SortOrder: mapstore.SortOrderAscending, FilterPartitions: []string{}}, "",
		)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 1 {
			t.Fatalf("expected 1 file, got %d", len(got))
		}
		if nextPageToken != "" {
			t.Fatalf("expected no next page token, got %q", nextPageToken)
		}
	})

	t.Run("FilenamePrefixFiltering_NoMatch", func(t *testing.T) {
		t.Parallel()
		baseDir := t.TempDir()
		partitions := []string{partition202306}
		files := []string{appleJSON, bananaJSON}
		createFiles(t, baseDir, partitions, files)
		mds, err := mapstore.NewMapDirectoryStore(
			baseDir,
			true,
			partitionProvider,
			jsonencdec.JSONEncoderDecoder{},
		)
		if err != nil {
			t.Fatalf("failed to create MapDirectoryStore: %v", err)
		}
		got, nextPageToken, err := mds.ListFiles(
			mapstore.ListingConfig{SortOrder: mapstore.SortOrderAscending, FilenamePrefix: "zzz"}, "",
		)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 0 {
			t.Fatalf("expected 0 files, got %v", got)
		}
		if nextPageToken != "" {
			t.Fatalf("expected no next page token, got %q", nextPageToken)
		}
	})

	t.Run("FilteredPagination_EmptyPartition", func(t *testing.T) {
		t.Parallel()
		baseDir := t.TempDir()
		files := []string{aJSON}
		createFiles(t, baseDir, []string{partition202307}, files)
		if err := os.MkdirAll(filepath.Join(baseDir, partition202302), 0o755); err != nil {
			t.Fatalf("failed to create partition dir: %v", err)
		}
		mds, err := mapstore.NewMapDirectoryStore(
			baseDir,
			true,
			partitionProvider,
			jsonencdec.JSONEncoderDecoder{},
			mapstore.WithDirPageSize(1),
		)
		if err != nil {
			t.Fatalf("failed to create MapDirectoryStore: %v", err)
		}
		got, nextPageToken, err := mds.ListFiles(
			mapstore.ListingConfig{
				SortOrder:        mapstore.SortOrderAscending,
				FilterPartitions: []string{partition202307, partition202302},
			},
			"",
		)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 1 || normalizeRel(got[0].BaseRelativePath) != normalizeRel(path.Join(partition202307, aJSON)) {
			t.Fatalf("expected [202307/a.json], got %v", got)
		}
		if nextPageToken != "" {
			t.Fatalf("expected no next page token, got %q", nextPageToken)
		}
	})

	t.Run("FilenamePrefixFiltering_EmptyPartition", func(t *testing.T) {
		t.Parallel()
		baseDir := t.TempDir()
		files := []string{appleJSON, bananaJSON}
		createFiles(t, baseDir, []string{partition202308}, files)
		if err := os.MkdirAll(filepath.Join(baseDir, partition202309), 0o755); err != nil {
			t.Fatalf("failed to create partition dir: %v", err)
		}
		mds, err := mapstore.NewMapDirectoryStore(
			baseDir,
			true,
			partitionProvider,
			jsonencdec.JSONEncoderDecoder{},
			mapstore.WithDirPageSize(1),
		)
		if err != nil {
			t.Fatalf("failed to create MapDirectoryStore: %v", err)
		}
		got, nextPageToken, err := mds.ListFiles(
			mapstore.ListingConfig{
				SortOrder:        mapstore.SortOrderAscending,
				FilterPartitions: []string{partition202308, partition202309},
				FilenamePrefix:   applePrefix,
			},
			"",
		)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 1 ||
			normalizeRel(got[0].BaseRelativePath) != normalizeRel(path.Join(partition202308, appleJSON)) {
			t.Fatalf("expected [202308/apple.json], got %v", got)
		}
		if nextPageToken != "" {
			t.Fatalf("expected no next page token, got %q", nextPageToken)
		}
	})
}

func TestMapDirectoryStore_RejectsPathSeparatorsInFileName(t *testing.T) {
	base := t.TempDir()
	mds, _ := mapstore.NewMapDirectoryStore(
		base,
		true,
		&dirpartition.NoPartitionProvider{},
		jsonencdec.JSONEncoderDecoder{},
	)

	for _, name := range []string{"a/b.json", `a\b.json`} {
		_, err := mds.OpenFile(mapstore.FileKey{FileName: name}, true, map[string]any{})
		if err == nil {
			t.Fatalf("expected error for filename %q", name)
		}
	}
}

func baseName(rel string) string {
	return path.Base(normalizeRel(rel))
}

// normalizeRel normalizes a relative path to use forward slashes so comparisons
// are stable across Windows/macOS/Linux.
func normalizeRel(p string) string {
	if p == "" {
		return ""
	}
	return path.Clean(filepath.ToSlash(p))
}

func createFiles(t *testing.T, baseDir string, partitions, files []string) {
	t.Helper()
	for _, partition := range partitions {
		dir := filepath.Join(baseDir, partition)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("failed to create partition dir: %v", err)
		}
		for _, file := range files {
			p := filepath.Join(dir, file)
			if err := os.WriteFile(p, []byte(`{"k":"v"}`), 0o600); err != nil {
				t.Fatalf("failed to write file: %v", err)
			}
		}
	}
}
