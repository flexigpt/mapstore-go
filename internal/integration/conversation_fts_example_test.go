package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ppipada/mapstore-go"
	"github.com/ppipada/mapstore-go/dirpartition"
	"github.com/ppipada/mapstore-go/ftsengine"
	"github.com/ppipada/mapstore-go/jsonencdec"
	"github.com/ppipada/mapstore-go/uuidv7filename"
)

// conversationMessage is a minimal message type for the example.
type conversationMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// conversation is a minimal conversation model used in the example.
type conversation struct {
	ID         string                `json:"id"`
	Title      string                `json:"title"`
	CreatedAt  time.Time             `json:"createdAt"`
	ModifiedAt time.Time             `json:"modifiedAt"`
	Messages   []conversationMessage `json:"messages"`
}

// ExampleMapDirectoryStore_full shows how to combine the directory store,
// UUIDv7-based filenames, JSON encoding helpers, and the FTS engine. It writes
// two conversations, builds an FTS index over the directory, and runs a search.
func ExampleMapDirectoryStore_full() {
	ctx := context.Background()
	baseDir, _ := os.MkdirTemp("", "mapstore_conversations")
	defer os.RemoveAll(baseDir)

	// Partition by yyyyMM derived from the UUIDv7 timestamp in the filename.
	pp := &dirpartition.MonthPartitionProvider{
		TimeFn: func(key mapstore.FileKey) (time.Time, error) {
			info, err := uuidv7filename.Parse(key.FileName)
			if err != nil {
				return time.Time{}, err
			}
			return info.Time, nil
		},
	}

	// Directory store for JSON conversations.
	mds, err := mapstore.NewMapDirectoryStore(
		baseDir,
		true,
		pp,
		jsonencdec.JSONEncoderDecoder{},
	)
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	defer func() {
		_ = mds.CloseAll()
	}()
	// Helper to write one conversation.
	writeConversation := func(id, title string, msgs []conversationMessage) error {
		now := time.Now().UTC()
		c := conversation{
			ID:         id,
			Title:      title,
			CreatedAt:  now,
			ModifiedAt: now,
			Messages:   msgs,
		}
		m, err := jsonencdec.StructWithJSONTagsToMap(c)
		if err != nil {
			return err
		}
		info, err := uuidv7filename.Build(id, title, "json")
		if err != nil {
			return err
		}
		return mds.SetFileData(mapstore.FileKey{FileName: info.FileName}, m)
	}

	// Two small conversations with different content.
	id1, _ := uuidv7filename.NewUUIDv7String()
	_ = writeConversation(id1, "First chat", []conversationMessage{
		{Role: "user", Content: "hello world"},
		{Role: "assistant", Content: "hi there"},
	})

	id2, _ := uuidv7filename.NewUUIDv7String()
	_ = writeConversation(id2, "Second chat", []conversationMessage{
		{Role: "user", Content: "searchable conversation about MapStore"},
		{Role: "assistant", Content: "MapStore stores JSON on disk"},
	})

	// FTS engine indexing title, user and assistant content, plus an mtime column.
	engine, err := ftsengine.NewEngine(ftsengine.Config{
		BaseDir:    baseDir,
		DBFileName: "conversations.fts.sqlite",
		Table:      "conversations",
		Columns: []ftsengine.Column{
			{Name: "title", Weight: 1},
			{Name: "text", Weight: 2},
			{Name: "mtime", Unindexed: true},
		},
	})
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	defer engine.Close()

	// ProcessFile converts a JSON conversation file into FTS columns.
	processFile := func(
		_ context.Context,
		baseDir, fullPath string,
		getPrev ftsengine.GetPrevCmp,
	) (ftsengine.SyncDecision, error) {
		if filepath.Ext(fullPath) != ".json" {
			return ftsengine.SyncDecision{Skip: true}, nil
		}
		st, err := os.Stat(fullPath)
		if err != nil {
			fmt.Println("stat error. skipping sync.", err)
			return ftsengine.SyncDecision{Skip: true}, nil
		}
		mtime := st.ModTime().UTC().Format(time.RFC3339Nano)
		if getPrev(fullPath) == mtime {
			return ftsengine.SyncDecision{ID: fullPath, Unchanged: true}, nil
		}
		raw, err := os.ReadFile(fullPath)
		if err != nil {
			fmt.Println("read error. skipping sync.", err)
			return ftsengine.SyncDecision{Skip: true}, nil
		}
		var c conversation
		if err := json.Unmarshal(raw, &c); err != nil {
			fmt.Println("json error. skipping sync.", err)
			return ftsengine.SyncDecision{Skip: true}, nil
		}
		var s strings.Builder
		for _, m := range c.Messages {
			s.WriteString(m.Content + "\n")
		}
		text := s.String()

		vals := map[string]string{
			"title": c.Title,
			"text":  text,
			"mtime": mtime,
		}
		return ftsengine.SyncDecision{
			ID:     fullPath,
			CmpOut: mtime,
			Vals:   vals,
		}, nil
	}

	// Build the index once over the directory.
	if _, err := ftsengine.SyncDirToFTS(ctx, engine, baseDir, "mtime", 1000, processFile); err != nil {
		fmt.Println("error:", err)
		return
	}

	// Search for conversations mentioning "MapStore".
	hits, _, err := engine.Search(ctx, "MapStore", "", 10)
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	for _, h := range hits {
		p := filepath.Base(h.ID)
		f, err := uuidv7filename.Parse(p)
		if err != nil {
			fmt.Println("error:", err)
			return
		}
		fmt.Println(f.Suffix)
	}

	// Output:
	// Second chat
}
