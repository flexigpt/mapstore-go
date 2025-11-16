package integration

import (
	"fmt"
	"os"
	"time"

	"github.com/ppipada/mapstore-go"
	"github.com/ppipada/mapstore-go/dirpartition"
	"github.com/ppipada/mapstore-go/jsonencdec"
)

// ExampleMapDirectoryStore mirrors the README quick-start snippet for
// managing JSON files inside a partitioned directory. It creates a temporary
// base dir, writes one profile file, and prints its contents.
func ExampleMapDirectoryStore() {
	baseDir, _ := os.MkdirTemp("", "mapstore_quickstart_dir")
	defer os.RemoveAll(baseDir)

	partitionProvider := &dirpartition.MonthPartitionProvider{
		TimeFn: func(fileKey mapstore.FileKey) (time.Time, error) { return time.Now(), nil },
	}

	mds, err := mapstore.NewMapDirectoryStore(
		baseDir,
		true,
		partitionProvider,
		jsonencdec.JSONEncoderDecoder{},
	)
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	defer func() {
		_ = mds.CloseAll()
	}()

	fileKey := mapstore.FileKey{FileName: "profile.json"}
	if err := mds.SetFileData(fileKey, map[string]any{"name": "Ada"}); err != nil {
		fmt.Println("error:", err)
		return
	}

	data, err := mds.GetFileData(fileKey, false)
	if err != nil {
		fmt.Println("error:", err)
		return
	}

	fmt.Println(data["name"])

	// Output:
	// Ada
}
