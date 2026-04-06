package integration

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/flexigpt/mapstore-go"
	"github.com/flexigpt/mapstore-go/internal/encdecutil"
	"github.com/flexigpt/mapstore-go/internal/maputil"
	"github.com/flexigpt/mapstore-go/jsonencdec"
	"github.com/flexigpt/mapstore-go/keyringencdec"
)

func TestMapFileStoreExtended(t *testing.T) {
	tests := []struct {
		name              string
		initialData       map[string]any
		keyEncDecs        map[string]mapstore.IOEncoderDecoder
		operations        []operation
		expectedFinalData map[string]any
	}{
		{
			name: "test with per-key encoders",
			initialData: map[string]any{
				"foo":    "hello",
				"bar":    "world",
				"parent": map[string]any{"child": "secret"},
			},
			keyEncDecs: map[string]mapstore.IOEncoderDecoder{
				// Example: "foo" => reverseStringEncoderDecoder{}, etc.
				"foo":          reverseStringEncoderDecoder{},
				"parent.child": reverseStringEncoderDecoder{},
			},
			operations: []operation{
				setKeyOperation{key: "foo", value: "new value for foo"},
				getKeyOperation{key: "foo", expectedValue: "new value for foo"},
				setKeyOperation{key: "bar", value: "new value for bar"},
				getKeyOperation{key: "bar", expectedValue: "new value for bar"},
			},
			expectedFinalData: map[string]any{
				"foo": "new value for foo",
				"bar": "new value for bar",
				"parent": map[string]any{
					"child": "secret",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a temporary file.
			tempDir := t.TempDir()
			filename := filepath.Join(tempDir, "mapstore_test.json")

			// Create store with initial data, using the new mapstore.WithValueEncDecGetter.
			store, err := mapstore.NewMapFileStore(
				filename,
				tt.initialData,
				jsonencdec.JSONEncoderDecoder{},
				mapstore.WithCreateIfNotExists(true),
				mapstore.WithValueEncDecGetter(func(pathSoFar []string) mapstore.IOEncoderDecoder {
					joined := strings.Join(pathSoFar, ".")
					if ed, ok := tt.keyEncDecs[joined]; ok {
						return ed
					}
					return nil
				}),
			)
			if err != nil {
				t.Fatalf("failed to create store: %v", err)
			}

			// Flush data.
			if err := store.Flush(); err != nil {
				t.Fatalf("failed to flush data: %v", err)
			}

			// Read raw data from file.
			rawData, err := os.ReadFile(filename)
			if err != nil {
				t.Fatalf("failed to read raw data from file: %v", err)
			}
			t.Logf("Raw data in file: %s", string(rawData))

			// Unmarshal raw data.
			var fileData map[string]any
			if err := json.Unmarshal(rawData, &fileData); err != nil {
				t.Fatalf("failed to unmarshal raw data: %v", err)
			}

			// Check that the values for the "encoded" keys are properly base64-encoded reversed strings.
			for key := range tt.keyEncDecs {
				keys := strings.Split(key, ".")
				val, err := maputil.GetValueAtPath(fileData, keys)
				if err != nil {
					t.Errorf("failed to get value at key %s in file data: %v", key, err)
					continue
				}

				strVal, ok := val.(string)
				if !ok {
					t.Errorf("expected string value at key %s in file data, got %T", key, val)
					continue
				}

				// The value should be a base64-encoded string.
				decodedBytes, err := base64.StdEncoding.DecodeString(strVal)
				if err != nil {
					t.Errorf("failed to base64-decode value at key %s: %v", key, err)
					continue
				}

				// The decoded bytes should be the reversed original string.
				reversedValue := string(decodedBytes)

				// Get the original value from initialData at the same key.
				originalVal, err := maputil.GetValueAtPath(tt.initialData, keys)
				if err != nil {
					t.Errorf("failed to get original value at key %s: %v", key, err)
					continue
				}

				origStrVal, ok := originalVal.(string)
				if !ok {
					t.Errorf(
						"expected string value at key %s in initial data, got %T",
						key,
						originalVal,
					)
					continue
				}

				expectedEncodedValue := reverseString(origStrVal)
				if reversedValue != expectedEncodedValue {
					t.Errorf(
						"encoded value at key %s does not match expected reversed value.\ngot: %s\nwant: %s",
						key,
						reversedValue,
						expectedEncodedValue,
					)
				}
			}

			// Now, create a new store from the file (using the same approach).
			newStore, err := mapstore.NewMapFileStore(
				filename,
				tt.initialData,
				jsonencdec.JSONEncoderDecoder{},
				mapstore.WithCreateIfNotExists(false),
				mapstore.WithValueEncDecGetter(func(pathSoFar []string) mapstore.IOEncoderDecoder {
					joined := strings.Join(pathSoFar, ".")
					if ed, ok := tt.keyEncDecs[joined]; ok {
						return ed
					}
					return nil
				}),
			)
			if err != nil {
				t.Fatalf("failed to create store from file: %v", err)
			}

			// Perform the user-defined operations.
			for _, op := range tt.operations {
				op.Execute(t, newStore)
			}

			// Flush store after operations.
			if err := newStore.Flush(); err != nil {
				t.Fatalf("failed to flush data after operations: %v", err)
			}

			// Check final in-memory data.
			finalData, err := newStore.GetAll(false)
			if err != nil {
				t.Errorf("Failed to get data err: %v", err)
			}
			if !reflect.DeepEqual(finalData, tt.expectedFinalData) {
				t.Errorf(
					"final data does not match expected.\ngot: %v\nwant:%v",
					finalData,
					tt.expectedFinalData,
				)
			}

			// Verify file contents again after operations.
			rawDataAfterOps, err := os.ReadFile(filename)
			if err != nil {
				t.Fatalf("failed to read raw data from file after operations: %v", err)
			}
			var fileDataAfterOps map[string]any
			if err := json.Unmarshal(rawDataAfterOps, &fileDataAfterOps); err != nil {
				t.Fatalf("failed to unmarshal raw data after operations: %v", err)
			}

			// Check that the values for the "encoded" keys remain properly encoded.
			for key := range tt.keyEncDecs {
				keys := strings.Split(key, ".")
				val, err := maputil.GetValueAtPath(fileDataAfterOps, keys)
				if err != nil {
					t.Errorf(
						"failed to get value at key %s in file data after operations: %v",
						key,
						err,
					)
					continue
				}

				strVal, ok := val.(string)
				if !ok {
					t.Errorf(
						"expected string value at key %s in file data after operations, got %T",
						key,
						val,
					)
					continue
				}

				decodedBytes, err := base64.StdEncoding.DecodeString(strVal)
				if err != nil {
					t.Errorf(
						"failed to base64-decode value at key %s after operations: %v",
						key,
						err,
					)
					continue
				}

				reversedValue := string(decodedBytes)

				// Compare to finalData’s in-memory value.
				finalVal, err := maputil.GetValueAtPath(tt.expectedFinalData, keys)
				if err != nil {
					t.Errorf("failed to get final value at key %s: %v", key, err)
					continue
				}
				finalStrVal, ok := finalVal.(string)
				if !ok {
					t.Errorf("expected string value at key %s in final data, got %T", key, finalVal)
					continue
				}

				expectedEncodedValue := reverseString(finalStrVal)
				if reversedValue != expectedEncodedValue {
					t.Errorf(
						"encoded value at key %s after operations does not match expected reversed value.\ngot: %s\nwant: %s",
						key,
						reversedValue,
						expectedEncodedValue,
					)
				}
			}
		})
	}
}

func TestMapFileStore(t *testing.T) {
	tempDir := t.TempDir()
	type testType struct {
		name              string
		filename          string
		defaultData       map[string]any
		createFile        bool
		fileContent       string
		options           []mapstore.FileOption
		expectError       bool
		expectedErrorText string
	}
	tests := []testType{
		{
			name:        "File does not exist, createIfNotExists true",
			filename:    filepath.Join(tempDir, "store1.json"),
			defaultData: map[string]any{"k": "v"},
			options:     []mapstore.FileOption{mapstore.WithCreateIfNotExists(true)},
			expectError: false,
		},
		{
			name:              "File does not exist, createIfNotExists false",
			filename:          filepath.Join(tempDir, "store2.json"),
			defaultData:       map[string]any{"k": "v"},
			options:           []mapstore.FileOption{mapstore.WithCreateIfNotExists(false)},
			expectError:       true,
			expectedErrorText: "does not exist",
		},
		{
			name:        "File exists with valid content",
			filename:    filepath.Join(tempDir, "store3.json"),
			defaultData: map[string]any{"k": "v"},
			createFile:  true,
			fileContent: `{"foo":"bar"}`,
			options:     []mapstore.FileOption{},
			expectError: false,
		},
		{
			name:        "File exists with invalid content",
			filename:    filepath.Join(tempDir, "store4.json"),
			defaultData: map[string]any{"k": "v"},
			createFile:  true,
			fileContent: `{invalid json}`,
			options:     []mapstore.FileOption{},
			expectError: true,
		},
		{
			name:        "File exists but cannot open",
			filename:    filepath.Join(tempDir, "store5.json"),
			defaultData: map[string]any{"k": "v"},
			createFile:  true,
			fileContent: `{"foo":"bar"}`,
			options:     []mapstore.FileOption{},
			expectError: true,
		},
	}

	runtNewMapFileStoreTestCase := func(t *testing.T, tt testType) {
		t.Helper()
		if tt.createFile {
			err := os.WriteFile(tt.filename, []byte(tt.fileContent), 0o600)
			if err != nil {
				t.Fatalf("[%s] Failed to create test file: %v", tt.name, err)
			}
		}

		if tt.name == "File exists but cannot open" {
			if runtime.GOOS == goosWindows {
				t.Skip("chmod(000) does not reliably prevent opening files on Windows")
			}
			// Create a file with no read permissions.
			err := os.Chmod(tt.filename, 0o000)
			if err != nil {
				t.Fatalf("[%s] Failed to change file permissions: %v", tt.name, err)
			}

			defer func() {
				// Ensure we can clean up later.
				_ = os.Chmod(tt.filename, 0o644)
			}()
		}

		_, err := mapstore.NewMapFileStore(tt.filename, tt.defaultData, jsonencdec.JSONEncoderDecoder{}, tt.options...)
		if tt.expectError {
			if err == nil {
				t.Errorf("[%s] Expected error but got nil", tt.name)
			} else if tt.expectedErrorText != "" && !strings.Contains(err.Error(), tt.expectedErrorText) {
				t.Errorf("[%s] Expected error containing '%s' but got '%v'", tt.name, tt.expectedErrorText, err)
			}
		} else {
			if err != nil {
				t.Errorf("[%s] Unexpected error: %v", tt.name, err)
			}
		}
	}

	for _, tt := range tests {
		runtNewMapFileStoreTestCase(t, tt)
	}
}

func TestMapFileStore_SetKey_GetKey(t *testing.T) {
	tempDir := t.TempDir()
	filename := filepath.Join(tempDir, "teststore.json")
	defaultData := map[string]any{"foo": "bar"}

	encoderDecoder1, err := keyringencdec.NewEncryptedStringValueEncoderDecoder("keyringencdec", "user1")
	if err != nil {
		t.Fatalf("getEncoderDecoder failed: %v", err)
	}
	encoderDecoder2, err := keyringencdec.NewEncryptedStringValueEncoderDecoder("keyringencdec", "user2")
	if err != nil {
		t.Fatalf("getEncoderDecoder failed: %v", err)
	}

	// Example: We'll return an EncoderDecoder for the paths "foo" and "parent.child".
	valueEncDecs := map[string]mapstore.IOEncoderDecoder{
		"foo":          encoderDecoder1,
		"parent.child": encoderDecoder2,
	}

	store, err := mapstore.NewMapFileStore(
		filename,
		defaultData,
		jsonencdec.JSONEncoderDecoder{},
		mapstore.WithCreateIfNotExists(true),
		// New approach: We pass a function that returns an EncoderDecoder depending on pathSoFar.
		mapstore.WithValueEncDecGetter(func(pathSoFar []string) mapstore.IOEncoderDecoder {
			joined := strings.Join(pathSoFar, ".")
			if ed, ok := valueEncDecs[joined]; ok {
				return ed
			}
			return nil
		}),
	)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}

	tests := []struct {
		name       string
		keys       []string
		value      any
		wantErrSet bool
		wantErrGet bool
	}{
		{
			name:  "Set and get simple key",
			keys:  []string{"foo"},
			value: "bar",
		},
		{
			name:  "Set and get nested key",
			keys:  []string{"parent", "child"},
			value: "grandson",
		},
		{
			name:  "Set and get deep nested key",
			keys:  []string{"grand", "parent", "child", "key"},
			value: true,
		},
		{
			name:       "Set empty key slice",
			keys:       []string{},
			value:      "value",
			wantErrSet: true,
		},
		{
			name:       "Set key with empty segment (like parent..child in dotted form)",
			keys:       []string{"parent", "", "child"},
			value:      "value",
			wantErrSet: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := store.SetKey(tt.keys, tt.value)
			if tt.wantErrSet {
				if err == nil {
					t.Errorf("[%s] Expected error in SetKey but got nil", tt.name)
				}
				return
			} else if err != nil {
				t.Errorf("[%s] Unexpected error in SetKey: %v", tt.name, err)
				return
			}

			got, err := store.GetKey(tt.keys)
			if tt.wantErrGet {
				if err == nil {
					t.Errorf("[%s] Expected error in GetKey but got nil", tt.name)
				}
				return
			} else {
				if err != nil {
					t.Errorf("[%s] Unexpected error in GetKey: %v", tt.name, err)
				} else if !reflect.DeepEqual(got, tt.value) {
					t.Errorf("[%s] GetKey returned %v, expected %v", tt.name, got, tt.value)
				}
			}
		})
	}
}

func TestMapFileStore_DeleteKey(t *testing.T) {
	tempDir := t.TempDir()
	filename := filepath.Join(tempDir, "teststore.json")
	// Pre-populate the store.
	initialData := map[string]any{
		"foo":    "bar",
		"parent": map[string]any{"child": "value"},
		"grand": map[string]any{
			"parent": map[string]any{"child": map[string]any{"key": "deep"}},
		},
		"nondeletable": "persist",
		"empty":        map[string]any{"parent": map[string]any{}},
		"list":         []any{1, 2, 3},
		"nonexistent":  nil,
		"another": map[string]any{
			"parent": map[string]any{"child1": "val1", "child2": "val2"},
		},
	}

	store, err := mapstore.NewMapFileStore(
		filename,
		initialData,
		jsonencdec.JSONEncoderDecoder{},
		mapstore.WithCreateIfNotExists(true),
	)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}

	tests := []struct {
		name       string
		keys       []string
		wantErr    bool
		checkExist bool
	}{
		{
			name:       "Delete simple key",
			keys:       []string{"foo"},
			checkExist: true,
		},
		{
			name:       "Delete nested key",
			keys:       []string{"parent", "child"},
			checkExist: true,
		},
		{
			name:       "Delete deep nested key",
			keys:       []string{"grand", "parent", "child", "key"},
			checkExist: true,
		},
		{
			name:    "Delete non-existent key",
			keys:    []string{"does", "not", "exist"},
			wantErr: false,
		},
		{
			name:    "Delete key with empty segment",
			keys:    []string{"parent", "", "child"},
			wantErr: false,
		},
		{
			name:       "Delete empty map",
			keys:       []string{"empty", "parent"},
			checkExist: true,
		},
		{
			name:       "Delete from map with multiple keys",
			keys:       []string{"another", "parent", "child1"},
			checkExist: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := store.DeleteKey(tt.keys)
			if tt.wantErr {
				if err == nil {
					t.Errorf("[%s] Expected error in DeleteKey but got nil", tt.name)
				}
				return
			} else if err != nil {
				t.Errorf("[%s] Unexpected error in DeleteKey: %v", tt.name, err)
				return
			}

			if tt.checkExist {
				_, err := store.GetKey(tt.keys)
				if err == nil {
					t.Errorf(
						"[%s] Expected key %v to be deleted, but it still exists",
						tt.name,
						tt.keys,
					)
				}
			}
		})
	}
}

func TestMapFileStore_SetAll_GetAll(t *testing.T) {
	tempDir := t.TempDir()
	filename := filepath.Join(tempDir, "teststore.json")
	defaultData := map[string]any{"foo": "bar"}
	store, err := mapstore.NewMapFileStore(
		filename,
		defaultData,
		jsonencdec.JSONEncoderDecoder{},
		mapstore.WithCreateIfNotExists(true),
		mapstore.WithFileAutoFlush(true),
	)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}

	tests := []struct {
		name          string
		data          map[string]any
		expectedData  map[string]any
		modifyData    bool
		expectedAfter map[string]any
	}{
		{
			name: "Set and get all simple data",
			data: map[string]any{
				"key1": "value1",
				"key2": 2,
			},
			expectedData: map[string]any{
				"key1": "value1",
				"key2": 2,
			},
		},
		{
			name: "Set and get all nested data",
			data: map[string]any{
				"parent": map[string]any{
					"child": "value",
				},
			},
			expectedData: map[string]any{
				"parent": map[string]any{
					"child": "value",
				},
			},
		},
		{
			name: "Set and get all empty data",
			data: map[string]any{},
		},
		{
			name: "Modify returned data should not affect store",
			data: map[string]any{
				"original": "value",
			},
			expectedData: map[string]any{
				"original": "value",
			},
			modifyData: true,
			expectedAfter: map[string]any{
				"original": "value",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset store to default before each subtest.
			err := store.Reset()
			if err != nil {
				t.Fatalf("[%s] Failed to reset store: %v", tt.name, err)
			}

			err = store.SetAll(tt.data)
			if err != nil {
				t.Errorf("[%s] SetAll failed: %v", tt.name, err)
				return
			}

			got, err := store.GetAll(false)
			if err != nil {
				t.Errorf("[%s] Failed to get data: %v", tt.name, err)
				return
			}

			if !deepEqual(got, tt.expectedData) {
				t.Errorf("[%s] GetAll returned %v, expected %v", tt.name, got, tt.expectedData)
			}

			if tt.modifyData {
				got["original"] = "modified"
				gotAfterModification, err := store.GetAll(false)
				if err != nil {
					t.Errorf("Failed to get data err: %v", err)
				}
				if !deepEqual(gotAfterModification, tt.expectedAfter) {
					t.Errorf(
						"[%s] Store data modified after external change: got %v, expected %v",
						tt.name,
						gotAfterModification,
						tt.expectedAfter,
					)
				}
			}
		})
	}
}

func TestMapFileStore_DeleteAll(t *testing.T) {
	tempDir := t.TempDir()
	filename := filepath.Join(tempDir, "teststore.json")
	defaultData := map[string]any{"foo": "bar"}
	store, err := mapstore.NewMapFileStore(
		filename,
		defaultData,
		jsonencdec.JSONEncoderDecoder{},
		mapstore.WithCreateIfNotExists(true),
	)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}

	// Re-populate the store.
	initialData := map[string]any{
		"key1": "value1",
		"key2": 2,
		"key3": true,
	}
	err = store.SetAll(initialData)
	if err != nil {
		t.Fatalf("Failed to set initial data: %v", err)
	}

	err = store.Reset()
	if err != nil {
		t.Fatalf("Reset failed: %v", err)
	}

	got, err := store.GetAll(false)
	if err != nil {
		t.Errorf("Failed to get data err: %v", err)
	}
	if !reflect.DeepEqual(got, defaultData) {
		t.Errorf("Expected store to reset to defaultData %v, but got %v", defaultData, got)
	}
}

func TestMapFileStore_AutoFlush(t *testing.T) {
	tempDir := t.TempDir()
	filename := filepath.Join(tempDir, "teststore_autoflush.json")
	defaultData := map[string]any{"k": "v"}
	store, err := mapstore.NewMapFileStore(
		filename,
		defaultData,
		jsonencdec.JSONEncoderDecoder{},
		mapstore.WithCreateIfNotExists(true),
		mapstore.WithFileAutoFlush(true),
	)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}

	err = store.SetKey([]string{"foo"}, "bar")
	if err != nil {
		t.Fatalf("SetKey failed: %v", err)
	}

	// Reopen the store.
	store2, err := mapstore.NewMapFileStore(filename, defaultData, jsonencdec.JSONEncoderDecoder{})
	if err != nil {
		t.Fatalf("Failed to reopen store: %v", err)
	}

	val, err := store2.GetKey([]string{"foo"})
	if err != nil {
		t.Fatalf("GetKey failed: %v", err)
	}
	if val != "bar" {
		t.Errorf("Expected 'bar', got %v", val)
	}
}

func TestMapFileStore_NoAutoFlush(t *testing.T) {
	tempDir := t.TempDir()
	filename := filepath.Join(tempDir, "teststore_noautoflush.json")
	defaultData := map[string]any{"k": "v"}
	store, err := mapstore.NewMapFileStore(
		filename,
		defaultData,
		jsonencdec.JSONEncoderDecoder{},
		mapstore.WithCreateIfNotExists(true),
		mapstore.WithFileAutoFlush(false),
	)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}

	err = store.SetKey([]string{"foo"}, "bar")
	if err != nil {
		t.Fatalf("SetKey failed: %v", err)
	}

	// Reopen the store.
	store2, err := mapstore.NewMapFileStore(filename, defaultData, jsonencdec.JSONEncoderDecoder{})
	if err != nil {
		t.Fatalf("Failed to reopen store: %v", err)
	}

	_, err = store2.GetKey([]string{"foo"})
	if err == nil {
		t.Errorf("Expected error getting 'foo' from store2 as it should not be saved yet")
	}

	// Now flush and reopen.
	err = store.Flush()
	if err != nil {
		t.Fatalf("Flush failed: %v", err)
	}

	store3, err := mapstore.NewMapFileStore(filename, defaultData, jsonencdec.JSONEncoderDecoder{})
	if err != nil {
		t.Fatalf("Failed to reopen store after save: %v", err)
	}

	val, err := store3.GetKey([]string{"foo"})
	if err != nil {
		t.Fatalf("GetKey failed: %v", err)
	}
	if val != "bar" {
		t.Errorf("Expected 'bar', got %v", val)
	}
}

func TestMapFileStorePermissionErrorCases(t *testing.T) {
	if runtime.GOOS == goosWindows {
		t.Skip("chmod does not reliably prevent opening files on Windows")
	}
	tempDir := t.TempDir()
	filename := filepath.Join(tempDir, "teststore_errors.json")
	defaultData := map[string]any{"k": "v"}
	store, err := mapstore.NewMapFileStore(
		filename,
		defaultData,
		jsonencdec.JSONEncoderDecoder{},
		mapstore.WithCreateIfNotExists(true),
	)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}

	// Simulate Save error by making the file unwritable.
	err = os.Chmod(filename, 0o444)
	if err != nil {
		t.Fatalf("Failed to change file permissions: %v", err)
	}
	ch := func() { _ = os.Chmod(filename, 0o644) }
	defer ch()

	err = store.SetKey([]string{"foo"}, "bar")
	if err == nil {
		t.Errorf("Expected error in SetKey due to unwritable file, but got nil")
	}

	// Simulate Decode error by writing invalid data into the file.
	err = os.Chmod(filename, 0o666)
	if err != nil {
		t.Fatalf("Failed to change file permissions: %v", err)
	}
	err = os.WriteFile(filename, []byte(`invalid json`), 0o600)
	if err != nil {
		t.Fatalf("Failed to write invalid data to file: %v", err)
	}

	_, err = mapstore.NewMapFileStore(filename, defaultData, jsonencdec.JSONEncoderDecoder{})
	if err == nil {
		t.Errorf("Expected error when loading store from invalid data, but got nil")
	}
}

func TestMapFileStore_NestedStructures(t *testing.T) {
	tempDir := t.TempDir()
	filename := filepath.Join(tempDir, "teststore_nested.json")
	defaultData := map[string]any{"k": "v"}
	store, err := mapstore.NewMapFileStore(
		filename,
		defaultData,
		jsonencdec.JSONEncoderDecoder{},
		mapstore.WithCreateIfNotExists(true),
	)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}

	// Set nested data.
	tests := []struct {
		name      string
		keys      []string
		value     any
		expectErr bool
	}{
		{
			name:  "Set nested map",
			keys:  []string{"parent", "child"},
			value: "value",
		},
		{
			name:  "Set nested map with existing parent",
			keys:  []string{"parent", "anotherChild"},
			value: 123,
		},
		{
			name:  "Set deep nested map",
			keys:  []string{"grand", "parent", "child"},
			value: true,
		},
		{
			name:  "Set value where intermediate is not a map",
			keys:  []string{"parent", "child", "key"},
			value: "invalid",
			// 'parent.child' is a string, cannot set 'key' under it.
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := store.SetKey(tt.keys, tt.value)
			if tt.expectErr {
				if err == nil {
					t.Errorf("[%s] Expected error in SetKey but got nil", tt.name)
				}
				return
			} else if err != nil {
				t.Errorf("[%s] SetKey failed: %v", tt.name, err)
				return
			}

			got, err := store.GetKey(tt.keys)
			if err != nil {
				t.Errorf("[%s] GetKey failed: %v", tt.name, err)
				return
			}
			if !reflect.DeepEqual(got, tt.value) {
				t.Errorf("[%s] GetKey returned %v, expected %v", tt.name, got, tt.value)
			}
		})
	}
}

// TestMapFileStore_KeyEncodingDecoding demonstrates how keys are encoded/decoded via mapstore.WithKeyEncDecGetter.
// We run table-driven sub-tests that verify:
//  1. We can set nested keys, then upon store reload, retrieve them with the *original* plain path.
//  2. If the on-disk data is corrupted (invalid base64 in a key), we get an error when loading.
//  3. Partial path coverage vs. all path coverage, etc.
func TestMapFileStore_KeyEncodingDecoding(t *testing.T) {
	tempDir := t.TempDir()
	filename := filepath.Join(tempDir, "teststore_key_jsonencdec.json")

	// We'll define a function that returns mockB64KeyEncDec for all paths,
	// i.e. it encodes/decodes every key in the store.
	keyEncDecGetter := func(pathSoFar []string) mapstore.StringEncoderDecoder {
		// Always return the base64 encoder/decoder.
		return encdecutil.Base64StringEncoderDecoder{}
	}

	// Create store with default data and our KeyEncDec.
	defaultData := map[string]any{"plainKey": "plainVal"}
	store, err := mapstore.NewMapFileStore(
		filename,
		defaultData,
		jsonencdec.JSONEncoderDecoder{},
		mapstore.WithCreateIfNotExists(true),
		mapstore.WithKeyEncDecGetter(keyEncDecGetter),
	)
	if err != nil {
		t.Fatalf("Failed to create store with keyEncDec: %v", err)
	}

	t.Run("Set and Get with Key Encoding - Nested Paths", func(t *testing.T) {
		testCases := []struct {
			name       string
			keys       []string
			value      any
			wantErrSet bool
			wantErrGet bool
		}{
			{
				name:  "simple top-level key",
				keys:  []string{"hello"},
				value: "world",
			},
			{
				name:  "nested key with two levels",
				keys:  []string{"level1", "level2"},
				value: 123,
			},
			{
				name:  "deep nested key",
				keys:  []string{"grandlevel", "parentlevel", "childlevel"},
				value: true,
			},
			{
				name:  "empty key slice",
				keys:  []string{},
				value: "value",
				// Cannot set root.
				wantErrSet: true,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				err := store.SetKey(tc.keys, tc.value)
				if tc.wantErrSet {
					if err == nil {
						t.Errorf("[%s] Expected error in SetKey but got nil", tc.name)
					}
					return
				} else if err != nil {
					t.Errorf("[%s] Unexpected error in SetKey: %v", tc.name, err)
					return
				}

				gotVal, err := store.GetKey(tc.keys)
				if tc.wantErrGet {
					if err == nil {
						t.Errorf("[%s] Expected error in GetKey but got nil", tc.name)
					}
					return
				}

				if err != nil {
					t.Errorf("[%s] Unexpected error in GetKey: %v", tc.name, err)
					return
				}
				if !reflect.DeepEqual(gotVal, tc.value) {
					t.Errorf("[%s] GetKey returned %v, expected %v", tc.name, gotVal, tc.value)
				}
			})
		}
	})

	t.Run("Reloading the Store Should Decode Keys Correctly", func(t *testing.T) {
		// We set some keys in the previous sub-test. Let's close the store,
		// reopen it, and ensure we get the same data with the same key paths.
		err := store.Flush()
		if err != nil {
			t.Fatalf("Unexpected error while flushing: %v", err)
		}
		store2, err := mapstore.NewMapFileStore(
			filename,
			defaultData,
			jsonencdec.JSONEncoderDecoder{},
			mapstore.WithKeyEncDecGetter(keyEncDecGetter),
		)
		if err != nil {
			t.Fatalf("Failed to reopen store after saving: %v", err)
		}

		// Try retrieving some keys we set in the previous sub-test.
		keysToCheck := [][]string{
			{"hello"},
			{"level1", "level2"},
			{"grandlevel", "parentlevel", "childlevel"},
		}
		for _, k := range keysToCheck {
			val, err := store2.GetKey(k)
			if err != nil {
				t.Errorf("GetKey(%v) failed after reopen: %v", k, err)
			} else {
				t.Logf("After reload, got key %v => %v (ok)", k, val)
			}
		}
	})

	t.Run("Invalid Base64 Key on Disk - Load Should Fail", func(t *testing.T) {
		// We'll forcibly write a key that is not valid base64 into the JSON file, to mimic a corrupt on-disk scenario.
		// Then, a new store load should fail because decode will error out on that key.
		err := store.Flush()
		if err != nil {
			t.Fatalf("Unexpected error while flushing: %v", err)
		}

		// Overwrite the store file with one invalid key (like: {"bad-base64??===": "someVal"}).
		invalidJSON := `{"bad-base64??===": "someVal"}`
		err = os.WriteFile(filename, []byte(invalidJSON), 0o600)
		if err != nil {
			t.Fatalf("Failed to write invalid base64 key to file: %v", err)
		}

		// Now attempt to open a new store that uses the same KeyEncDec.
		_, err = mapstore.NewMapFileStore(
			filename,
			defaultData,
			jsonencdec.JSONEncoderDecoder{},
			mapstore.WithKeyEncDecGetter(keyEncDecGetter),
		)
		if err == nil {
			t.Errorf("Expected error when loading invalid base64 key from disk, but got nil")
		} else {
			t.Logf("Got expected error: %v", err)
		}
	})
}

// Below is an example test enhancement for SetAll/GetAll with key encoding.
// We'll ensure that keys are also re-encoded/de-encoded properly when using SetAll.
func TestMapFileStore_SetAll_KeyEncDec(t *testing.T) {
	tempDir := t.TempDir()
	filename := filepath.Join(tempDir, "teststore_setall_keyjsonencdec.json")

	keyEncDecGetter := func(pathSoFar []string) mapstore.StringEncoderDecoder {
		return encdecutil.Base64StringEncoderDecoder{}
	}
	defaultData := map[string]any{"default": "val"}

	store, err := mapstore.NewMapFileStore(
		filename,
		defaultData,
		jsonencdec.JSONEncoderDecoder{},
		mapstore.WithCreateIfNotExists(true),
		mapstore.WithKeyEncDecGetter(keyEncDecGetter),
	)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}

	tests := []struct {
		name        string
		data        map[string]any
		keysToCheck [][]string
	}{
		{
			name: "simple set of keys",
			data: map[string]any{
				"alpha":   "bravo",
				"charlie": "delta",
			},
			keysToCheck: [][]string{{"alpha"}, {"charlie"}},
		},
		{
			name: "nested maps inside setAll",
			data: map[string]any{
				"root": map[string]any{
					"inner": "val1",
					"deep": map[string]any{
						"level": 42,
					},
				},
			},
			keysToCheck: [][]string{
				{"root", "inner"},
				{"root", "deep", "level"},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := store.SetAll(tc.data)
			if err != nil {
				t.Errorf("[%s] SetAll error: %v", tc.name, err)
				return
			}
			// Now retrieve them to ensure they're set properly.
			for _, k := range tc.keysToCheck {
				got, err := store.GetKey(k)
				if err != nil {
					t.Errorf("[%s] GetKey(%v) error: %v", tc.name, k, err)
					continue
				}
				wantVal := getValueAtPath(tc.data, k)
				if !reflect.DeepEqual(got, wantVal) {
					t.Errorf("[%s] mismatch: got %v, want %v for path %v", tc.name, got, wantVal, k)
				}
			}
		})
	}
}

// Order & fan-out: multiple listeners receive identical sequence.
func TestEvents_MultipleListeners_IdenticalOrder(t *testing.T) {
	tmp := t.TempDir()
	f := filepath.Join(tmp, "multi.json")

	var aMu, bMu sync.Mutex
	var recA, recB []mapstore.FileEvent

	lA := func(e mapstore.FileEvent) { aMu.Lock(); recA = append(recA, noTime(e)); aMu.Unlock() }
	lB := func(e mapstore.FileEvent) { bMu.Lock(); recB = append(recB, noTime(e)); bMu.Unlock() }

	st := openStore(f, mapstore.WithFileListeners(lA, lB))

	_ = st.SetAll(map[string]any{"x": 1})
	_ = st.SetKey([]string{"x"}, 2)
	_ = st.DeleteKey([]string{"x"})
	_ = st.Reset()

	aMu.Lock()
	bMu.Lock()
	defer aMu.Unlock()
	defer bMu.Unlock()

	if !reflect.DeepEqual(recA, recB) {
		t.Fatalf("listeners saw different events:\nA: %#v\nB: %#v", recA, recB)
	}
	if len(recA) != 4 {
		t.Fatalf("want 4 events, got %d", len(recA))
	}
}

// AutoFlush = false -> event fires, disk unchanged until Flush().
func TestEvents_AutoFlushFalse(t *testing.T) {
	const key = "foo"
	const val = "bar"
	tmp := t.TempDir()
	f := filepath.Join(tmp, "noflush.json")

	var ev mapstore.FileEvent
	st := openStore(
		f,
		mapstore.WithFileAutoFlush(false),
		mapstore.WithFileListeners(func(e mapstore.FileEvent) { ev = noTime(e) }),
	)

	if err := st.SetKey([]string{key}, val); err != nil {
		t.Fatalf("SetKey: %v", err)
	}
	if ev.Op != mapstore.OpSetKey || ev.NewValue != val {
		t.Fatalf("unexpected event %+v", ev)
	}

	// Reopen - change should NOT be on disk.
	reopen := openStore(f)
	if _, err := reopen.GetKey([]string{key}); err == nil {
		t.Fatalf("value persisted although autoFlush was off")
	}

	// Now flush and check again.
	if err := st.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}
	reopen2 := openStore(f)
	got, err := reopen2.GetKey([]string{key})
	if err != nil || got != val {
		t.Fatalf("after flush expected bar, got %v err %v", got, err)
	}
}

// Data snapshot matches store state for every op.
func TestEvents_DataSnapshotMatchesStore(t *testing.T) {
	tmp := t.TempDir()
	f := filepath.Join(tmp, "datasnap.json")

	var ev mapstore.FileEvent
	st := openStore(f, mapstore.WithFileListeners(func(e mapstore.FileEvent) { ev = noTime(e) }))

	steps := []struct {
		name string
		op   func() error
	}{
		{
			"SetAll",
			func() error { return st.SetAll(map[string]any{"a": 1}) },
		},
		{
			"SetKey",
			func() error { return st.SetKey([]string{"b"}, 2) },
		},
		{
			"DeleteKey",
			func() error { return st.DeleteKey([]string{"a"}) },
		},
		{
			"Reset",
			func() error { return st.Reset() },
		},
	}

	for _, stp := range steps {
		t.Run(stp.name, func(t *testing.T) {
			if err := stp.op(); err != nil {
				t.Fatalf("op err: %v", err)
			}
			want, _ := st.GetAll(false)
			if !reflect.DeepEqual(ev.Data, want) {
				t.Fatalf("event.Data diverged\n have %v\n want %v", ev.Data, want)
			}
		})
	}
}

// Concurrency - 100 goroutines issue SetKey; every event captured.
func TestEvents_ConcurrentWrites(t *testing.T) {
	tmp := t.TempDir()
	f := filepath.Join(tmp, "concurrent.json")

	var mu sync.Mutex
	var evs []mapstore.FileEvent
	st := openStore(
		f,
		mapstore.WithFileListeners(func(e mapstore.FileEvent) {
			mu.Lock()
			evs = append(evs, noTime(e))
			mu.Unlock()
		}),
	)

	const n = 100
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_ = st.SetKey([]string{"k"}, i)
		}(i)
	}
	wg.Wait()

	// Ensure we have exactly n events of type OpSetKey.
	mu.Lock()
	defer mu.Unlock()
	if len(evs) != n {
		t.Fatalf("expected %d events, got %d", n, len(evs))
	}
	for _, e := range evs {
		if e.Op != mapstore.OpSetKey {
			t.Fatalf("unexpected op %v", e.Op)
		}
	}
	// Final store value should equal last event.NewValue.
	last := evs[len(evs)-1].NewValue
	got, _ := st.GetKey([]string{"k"})
	if !reflect.DeepEqual(got, last) {
		t.Fatalf("store value %v, last event %v mismatch", got, last)
	}
}

// Panic isolation.
func TestEvents_PanicListener_DoesNotBreakNextListeners(t *testing.T) {
	tmp := t.TempDir()
	f := filepath.Join(tmp, "isolate.json")

	var called bool
	lGood := func(mapstore.FileEvent) { called = true }
	lBad := func(mapstore.FileEvent) { panic("bad") }

	st := openStore(f, mapstore.WithFileListeners(lBad, lGood))

	if err := st.SetKey([]string{"x"}, 1); err != nil {
		t.Fatalf("SetKey: %v", err)
	}
	if !called {
		t.Fatalf("good listener was not invoked after panic in bad listener")
	}
}

func TestMapFileStore_FlushTwice(t *testing.T) {
	tmp := t.TempDir()
	f := filepath.Join(tmp, "a.json")

	st, err := mapstore.NewMapFileStore(
		f, map[string]any{}, jsonencdec.JSONEncoderDecoder{},
		mapstore.WithCreateIfNotExists(true),
		mapstore.WithFileAutoFlush(true),
	)
	if err != nil {
		t.Fatal(err)
	}

	if err := st.SetKey([]string{"k"}, "v1"); err != nil {
		t.Fatal(err)
	}
	if err := st.SetKey([]string{"k"}, "v2"); err != nil {
		t.Fatal(err)
	}
}

func openStore(p string, opts ...mapstore.FileOption) *mapstore.MapFileStore {
	s, err := mapstore.NewMapFileStore(
		p,
		map[string]any{},
		jsonencdec.JSONEncoderDecoder{},
		append(opts, mapstore.WithCreateIfNotExists(true))...,
	)
	if err != nil {
		panic(err)
	}
	return s
}

// getValueAtPath is a helper to retrieve the value from a map at the given nested path.
// This is only used in our test expansions for quick checking inside the table-driven tests.
func getValueAtPath(m map[string]any, path []string) any {
	current := any(m)
	for _, p := range path {
		subMap, ok := current.(map[string]any)
		if !ok {
			return nil
		}
		current = subMap[p]
	}
	return current
}

func noTime(e mapstore.FileEvent) mapstore.FileEvent { e.Timestamp = time.Time{}; return e }

// deepEqual is a simple helper function to compare two any values for equality.
func deepEqual(a, b any) bool {
	switch aVal := a.(type) {
	case map[string]any:
		bVal, ok := b.(map[string]any)
		if !ok || len(aVal) != len(bVal) {
			return false
		}
		for k, v := range aVal {
			if !deepEqual(v, bVal[k]) {
				return false
			}
		}
		return true
	case []any:
		bVal, ok := b.([]any)
		if !ok || len(aVal) != len(bVal) {
			return false
		}
		for i := range aVal {
			if !deepEqual(aVal[i], bVal[i]) {
				return false
			}
		}
		return true
	default:
		return a == b
	}
}
