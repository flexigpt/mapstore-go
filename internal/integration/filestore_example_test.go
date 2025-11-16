package integration

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/ppipada/mapstore-go"
	"github.com/ppipada/mapstore-go/jsonencdec"
)

// ExampleMapFileStore mirrors the README quick-start snippet for a
// single JSON-backed file store. It creates a temporary config file,
// sets a nested key, then prints the resulting map entry.
func ExampleMapFileStore() {
	tmp, _ := os.MkdirTemp("", "mapstore_quickstart_file")
	defer os.RemoveAll(tmp)

	file := filepath.Join(tmp, "config.json")

	store, err := mapstore.NewMapFileStore(
		file,
		map[string]any{"env": "dev"},
		jsonencdec.JSONEncoderDecoder{},
		mapstore.WithCreateIfNotExists(true),
	)
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	defer store.Close()

	if err := store.SetKey([]string{"features", "logging"}, true); err != nil {
		fmt.Println("error:", err)
		return
	}

	data, err := store.GetAll(false)
	if err != nil {
		fmt.Println("error:", err)
		return
	}

	fmt.Println(data["features"])

	// Output:
	// map[logging:true]
}

// Basic event flow
// Sets a couple of keys, deletes them, then resets the file.  We attach a single
// listener that records every event and print a short, deterministic summary.
func Example_events_basicFlow() {
	tmp, _ := os.MkdirTemp("", "fs_example1")
	defer os.RemoveAll(tmp)
	file := filepath.Join(tmp, "store.json")

	// Record every event we receive.
	var mu sync.Mutex
	var got []mapstore.FileEvent
	rec := func(e mapstore.FileEvent) {
		mu.Lock()
		defer mu.Unlock()

		// Strip volatile fields so that the output is deterministic.
		e.File = ""
		e.Timestamp = time.Time{}
		got = append(got, e)
	}

	store, _ := mapstore.NewMapFileStore(
		file,
		// No default data.
		nil,
		jsonencdec.JSONEncoderDecoder{},
		mapstore.WithCreateIfNotExists(true),
		mapstore.WithFileListeners(rec),
	)

	_ = store.SetAll(map[string]any{"a": 1})
	_ = store.SetKey([]string{"a"}, 2)
	_ = store.DeleteKey([]string{"a"})
	_ = store.Reset()

	// Pretty-print the recorded events.
	mu.Lock()
	for _, ev := range got {
		switch ev.Op {
		case mapstore.OpSetFile:
			fmt.Printf("%s -> %v\n", ev.Op, ev.Data)
		case mapstore.OpResetFile:
			fmt.Printf("%s\n", ev.Op)
		case mapstore.OpDeleteFile, mapstore.OpSetKey, mapstore.OpDeleteKey:
			fmt.Printf("%s %v  old=%v  new=%v\n",
				ev.Op, ev.Keys, ev.OldValue, ev.NewValue)
		}
	}
	mu.Unlock()

	// Output:
	// setFile -> map[a:1]
	// setKey [a]  old=1  new=2
	// deleteKey [a]  old=2  new=<nil>
	// resetFile
}

// AutoFlush =false
// Shows that events are still delivered immediately, but the mutation only
// reaches disk after an explicit Flush().
func Example_events_autoFlush() {
	tmp, _ := os.MkdirTemp("", "fs_example2")
	defer os.RemoveAll(tmp)
	file := filepath.Join(tmp, "store.json")

	var last mapstore.FileEvent
	listener := func(e mapstore.FileEvent) { last = e }

	st, _ := mapstore.NewMapFileStore(
		file,
		nil,
		jsonencdec.JSONEncoderDecoder{},
		mapstore.WithCreateIfNotExists(true),
		mapstore.WithFileAutoFlush(false),
		mapstore.WithFileListeners(listener),
	)

	_ = st.SetKey([]string{"unsaved"}, 123)
	fmt.Println("event op:", last.Op)

	// Re-open the file - the key is not there yet.
	reopen1, _ := mapstore.NewMapFileStore(file, nil, jsonencdec.JSONEncoderDecoder{})
	if _, err := reopen1.GetKey([]string{"unsaved"}); err != nil {
		fmt.Println("not on disk yet")
	}

	// Flush and try again.
	_ = st.Flush()
	reopen2, _ := mapstore.NewMapFileStore(file, nil, jsonencdec.JSONEncoderDecoder{})
	v, _ := reopen2.GetKey([]string{"unsaved"})
	fmt.Println("on disk after flush:", v)

	// Output:
	// event op: setKey
	// not on disk yet
	// on disk after flush: 123
}

// Panic isolation between listeners
// One listener panics; the second one must still be called.
func Example_events_panicIsolation() {
	tmp, _ := os.MkdirTemp("", "fs_example3")
	defer os.RemoveAll(tmp)
	file := filepath.Join(tmp, "store.json")

	bad := func(mapstore.FileEvent) { panic("boom") }
	var goodCalled bool
	good := func(mapstore.FileEvent) { goodCalled = true }

	st, _ := mapstore.NewMapFileStore(
		file,
		nil,
		jsonencdec.JSONEncoderDecoder{},
		mapstore.WithCreateIfNotExists(true),
		mapstore.WithFileListeners(bad, good),
	)

	_ = st.SetKey([]string{"x"}, 1)
	fmt.Println("good listener called:", goodCalled)

	// Output:
	// good listener called: true
}
