package ftsengine

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	testDupColumnName = "dup"
	testTag           = "tag"
	testHelloWorld    = "hello world"
	colBody           = "body"
	testSecond        = "second"
	testDBFileName    = "fts.sqlite"
	testDocsTable     = "docs"
	testHello         = "hello"
	testBar           = "bar"
	testDoc1          = "doc1"
	testDoc2          = "doc2"
	testDoc4          = "doc4"
	testSecret        = "secret"
	testNotFound      = "notfound"
)

func TestValidateConfigAndConstructor(t *testing.T) {
	t.Run("happy in-memory engine", func(t *testing.T) {
		e := newMemoryEngine(t) // Should not panic.
		e.Close()
	})
	t.Run("duplicate column names rejected", func(t *testing.T) {
		e, err := NewEngine(Config{
			BaseDir: MemoryDBBaseDir,
			Table:   "t",
			Columns: []Column{{Name: testDupColumnName}, {Name: testDupColumnName}},
		})
		if err == nil || !strings.Contains(err.Error(), "duplicate column") {
			t.Fatalf("expected duplicate column error, got %v", err)
		} else if e != nil {
			e.Close()
		}
	})
	t.Run("empty column name rejected", func(t *testing.T) {
		e, err := NewEngine(Config{
			BaseDir: MemoryDBBaseDir,
			Table:   "t",
			Columns: []Column{{Name: ""}},
		})
		if err == nil || !strings.Contains(err.Error(), "empty name") {
			t.Fatalf("expected empty column error, got %v", err)
		} else if e != nil {
			e.Close()
		}
	})
	t.Run("missing table name rejected", func(t *testing.T) {
		e, err := NewEngine(Config{
			BaseDir: MemoryDBBaseDir,
			Table:   "   ",
			Columns: []Column{{Name: "x"}},
		})
		if err == nil {
			t.Fatalf("want error for empty table name")
		} else if e != nil {
			e.Close()
		}
	})
}

func TestIsEmptyAndCRUD(t *testing.T) {
	e := newTestEngine(t)

	// New engine must be empty.
	isEmp, _ := e.IsEmpty(t.Context())
	if !isEmp {
		t.Fatal("new engine should be empty")
	}

	// Insert two documents.
	if err := e.Upsert(t.Context(), "doc/alpha", map[string]string{
		testColumnTitle: testHelloWorld,
		colBody:         "ignored",
	}); err != nil {
		t.Fatalf("upsert alpha: %v", err)
	}
	if err := e.Upsert(t.Context(), "doc/bravo", map[string]string{
		testColumnTitle: testSecond,
		colBody:         "hello world again",
	}); err != nil {
		t.Fatalf("upsert bravo: %v", err)
	}
	isEmp, _ = e.IsEmpty(t.Context())
	if isEmp {
		t.Fatal("index should not be empty after inserts")
	}

	// Search must hit two documents.
	hits, next, err := e.Search(t.Context(), testHello, "", 10)
	if err != nil || len(hits) != 2 || next != "" {
		t.Fatalf("search expected 2 hits, got %d (next=%q, err=%v)",
			len(hits), next, err)
	}

	// Update one, delete the other.
	if err := e.Upsert(t.Context(), "doc/alpha", map[string]string{
		testColumnTitle: "updated",
		colBody:         "",
	}); err != nil {
		t.Fatalf("update alpha: %v", err)
	}
	_ = e.Delete(t.Context(), "doc/bravo")

	// No document should match the old term.
	hits, _, _ = e.Search(t.Context(), testHello, "", 10)
	if len(hits) != 0 {
		t.Fatalf("expected 0 hits, got %d", len(hits))
	}
}

func TestWeightRanking(t *testing.T) {
	e := newTestEngine(t)

	_ = e.Upsert(t.Context(), "1", map[string]string{
		testColumnTitle: "alpha winner",
		colBody:         "",
	})
	_ = e.Upsert(t.Context(), "2", map[string]string{
		testColumnTitle: "",
		colBody:         "alpha only in body",
	})

	hits, _, err := e.Search(t.Context(), "alpha", "", 10)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(hits) != 2 {
		t.Fatalf("want 2 hits, got %d", len(hits))
	}
	if hits[0].ID != "1" {
		t.Fatalf("title-match should rank first, got %q", hits[0].ID)
	}
	if hits[0].Score >= hits[1].Score {
		t.Fatalf("bm25 score ordering unexpected: %.3f >= %.3f",
			hits[0].Score, hits[1].Score)
	}
}

func TestSearchPaginationAndTokenHandling(t *testing.T) {
	e := newTestEngine(t)

	// Insert 15 documents containing the term testValueFoo.
	for i := range 15 {
		_ = e.Upsert(t.Context(), "id"+strconv.Itoa(i), map[string]string{
			testColumnTitle: "",
			colBody:         "foo bar",
		})
	}

	token := ""
	seen := map[string]bool{}
	total := 0

	for page := 0; ; page++ {
		hits, next, err := e.Search(t.Context(), testValueFoo, token, 6)
		if err != nil {
			t.Fatalf("page %d: %v", page, err)
		}
		for _, h := range hits {
			if seen[h.ID] {
				t.Fatalf("duplicate id %s across pages", h.ID)
			}
			seen[h.ID] = true
		}
		total += len(hits)
		if next == "" {
			// Last page must have 3 items.
			if len(hits) != 3 {
				t.Fatalf("last page size, want 3, got %d", len(hits))
			}
			break
		}
		if len(hits) != 6 {
			t.Fatalf("full pages must have 6 items, got %d", len(hits))
		}
		token = next
	}
	if total != 15 {
		t.Fatalf("expected 15 hits total, got %d", total)
	}

	t.Run("invalid base64 token is ignored", func(t *testing.T) {
		hits, _, err := e.Search(t.Context(), testValueFoo, "!!bad", 10)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(hits) == 0 {
			t.Fatalf("search should still return results")
		}
	})

	t.Run("malformed json token is ignored", func(t *testing.T) {
		bad := base64.StdEncoding.EncodeToString([]byte("{notjson"))
		hits, _, err := e.Search(t.Context(), testValueFoo, bad, 10)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(hits) == 0 {
			t.Fatalf("search should still return results")
		}
	})

	t.Run("pageSize ≤0 or >10k uses default", func(t *testing.T) {
		for _, sz := range []int{-5, 0, 20000} {
			hitsA, _, _ := e.Search(t.Context(), testValueFoo, "", sz)
			hitsB, _, _ := e.Search(t.Context(), testValueFoo, "", 10)
			if len(hitsA) != len(hitsB) {
				t.Fatalf("pageSize %d should fall back to default", sz)
			}
		}
	})
}

func TestEdgeCases(t *testing.T) {
	// Validation scenarios already covered in TestValidateConfigAndConstructor.
	e := newTestEngine(t)

	t.Run("empty docID rejected", func(t *testing.T) {
		if err := e.Upsert(t.Context(), "", map[string]string{testColumnTitle: "x"}); err == nil {
			t.Error("expected validation error for empty id")
		}
	})

	t.Run("delete unknown id returns nil error", func(t *testing.T) {
		if err := e.Delete(t.Context(), "does/not/exist"); err != nil {
			t.Errorf("delete unknown: %v", err)
		}
	})

	t.Run("row replacement keeps only one copy", func(t *testing.T) {
		if err := e.Upsert(t.Context(), testDupColumnName, map[string]string{colBody: "first"}); err != nil {
			t.Fatal(err)
		}
		if err := e.Upsert(t.Context(), testDupColumnName, map[string]string{colBody: testSecond}); err != nil {
			t.Fatal(err)
		}
		h, _, _ := e.Search(t.Context(), testSecond, "", 10)
		if len(h) != 1 || h[0].ID != testDupColumnName {
			t.Fatalf("replace failed, hits=%v", h)
		}
	})

	t.Run("IsEmpty resets after all deletes", func(t *testing.T) {
		_ = e.Delete(t.Context(), testDupColumnName)
		isEmp, _ := e.IsEmpty(t.Context())
		if !isEmp {
			t.Error("IsEmpty should be true after deleting last row")
		}
	})

	t.Run("token ignored on different query", func(t *testing.T) {
		_ = e.Upsert(t.Context(), "a1", map[string]string{testColumnTitle: "apple"})
		_ = e.Upsert(t.Context(), "a2", map[string]string{testColumnTitle: "apple"})

		h1, tok, _ := e.Search(t.Context(), "apple", "", 1)
		if len(h1) != 1 || tok == "" {
			t.Fatalf("setup failed, hits=%d token=%q", len(h1), tok)
		}

		// Use token with a different query. Offset must reset, so we get 0 hits.
		h2, _, _ := e.Search(t.Context(), "banana", tok, 1)
		if len(h2) != 0 {
			t.Fatalf("token should reset for new query, got %d hits", len(h2))
		}
	})
}

func TestSchemaChangeDropsPreviousData(t *testing.T) {
	tmp := t.TempDir()

	// First version with one column.
	cfgV1 := Config{
		BaseDir:    tmp,
		DBFileName: testDBFileName,
		Table:      testDocsTable,
		Columns:    []Column{{Name: colBody}},
	}
	e1, err := NewEngine(cfgV1)
	if err != nil {
		t.Fatalf("engine v1 init: %v", err)
	}
	if err := e1.Upsert(t.Context(), "x", map[string]string{colBody: testHello}); err != nil {
		t.Fatalf("insert v1: %v", err)
	}
	e1.Close()

	// Second version adds a column, which must change the checksum,
	// therefore the virtual table is recreated and previous rows vanish.
	cfgV2 := cfgV1
	cfgV2.Columns = append(cfgV2.Columns, Column{Name: testColumnTitle})
	e2, err := NewEngine(cfgV2)
	if err != nil {
		t.Fatalf("engine v2 init: %v", err)
	}
	defer e2.Close()

	empty, _ := e2.IsEmpty(t.Context())
	if !empty {
		t.Fatal("schema change should have dropped existing rows")
	}
}

func TestBatchUpsert_BasicAndEdgeCases(t *testing.T) {
	e := newBatchTestEngine(t)
	ctx := t.Context()

	t.Run("empty batch is no-op", func(t *testing.T) {
		if err := e.BatchUpsert(ctx, nil); err != nil {
			t.Fatalf("empty batch should not error: %v", err)
		}
	})

	t.Run("single doc insert", func(t *testing.T) {
		docs := map[string]map[string]string{
			testDoc1: {testColumnTitle: testValueFoo, colBody: testBar, testTag: "t1"},
		}
		if err := e.BatchUpsert(ctx, docs); err != nil {
			t.Fatalf("single doc batch upsert: %v", err)
		}
		hits, _, _ := e.Search(ctx, testValueFoo, "", 10)
		if len(hits) != 1 || hits[0].ID != testDoc1 {
			t.Fatalf("expected doc1, got %+v", hits)
		}
	})

	t.Run("multiple docs insert", func(t *testing.T) {
		docs := map[string]map[string]string{
			testDoc2: {testColumnTitle: testHello, colBody: "world", testTag: "t2"},
			"doc3":   {testColumnTitle: "goodbye", colBody: "moon", testTag: "t3"},
		}
		if err := e.BatchUpsert(ctx, docs); err != nil {
			t.Fatalf("multi doc batch upsert: %v", err)
		}
		hits, _, _ := e.Search(ctx, testHello, "", 10)
		if len(hits) != 1 || hits[0].ID != testDoc2 {
			t.Fatalf("expected doc2, got %+v", hits)
		}
	})

	t.Run("update existing doc", func(t *testing.T) {
		docs := map[string]map[string]string{
			testDoc2: {testColumnTitle: "updated", colBody: "world", testTag: "t2"},
		}
		if err := e.BatchUpsert(ctx, docs); err != nil {
			t.Fatalf("update doc2: %v", err)
		}
		hits, _, _ := e.Search(ctx, "updated", "", 10)
		if len(hits) != 1 || hits[0].ID != testDoc2 {
			t.Fatalf("expected updated doc2, got %+v", hits)
		}
	})

	t.Run("missing column values are empty", func(t *testing.T) {
		docs := map[string]map[string]string{
			testDoc4: {testColumnTitle: "only title"},
		}
		if err := e.BatchUpsert(ctx, docs); err != nil {
			t.Fatalf("missing columns: %v", err)
		}
		// Should be searchable by title but not by body or tag.
		hits, _, _ := e.Search(ctx, "only", "", 10)
		if len(hits) != 1 || hits[0].ID != testDoc4 {
			t.Fatalf("expected doc4, got %+v", hits)
		}
		hits, _, _ = e.Search(ctx, "t1", "", 10)
		for _, h := range hits {
			if h.ID == testDoc4 {
				t.Fatalf("doc4 should not match tag t1")
			}
		}
	})

	t.Run("unknown column names are ignored", func(t *testing.T) {
		docs := map[string]map[string]string{
			"doc5": {testColumnTitle: "x", "unknown": "y"},
		}
		if err := e.BatchUpsert(ctx, docs); err != nil {
			t.Fatalf("should ignore unknown column, got %v", err)
		}
	})

	t.Run("nil value map treated as empty strings", func(t *testing.T) {
		docs := map[string]map[string]string{
			"doc6": nil,
		}
		if err := e.BatchUpsert(ctx, docs); err != nil {
			t.Fatalf("nil value map: %v", err)
		}
		// Nothing should match, but operation must succeed.
	})

	t.Run("empty docID rejected", func(t *testing.T) {
		docs := map[string]map[string]string{
			"": {testColumnTitle: "no id"},
		}
		err := e.BatchUpsert(ctx, docs)
		if err == nil || !strings.Contains(err.Error(), "empty id") {
			t.Fatalf("expected error for empty id, got %v", err)
		}
	})
}

func TestBatchUpsert_ScaleAndStress(t *testing.T) {
	e := newBatchTestEngine(t)
	ctx := t.Context()

	const N = 5000
	docs := make(map[string]map[string]string, N)
	for i := range N {
		docs[fmt.Sprintf("id%04d", i)] = map[string]string{
			testColumnTitle: fmt.Sprintf("title %d", i),
			colBody:         fmt.Sprintf("body %d", i),
			testTag:         fmt.Sprintf("tag%d", i%10),
		}
	}
	if err := e.BatchUpsert(ctx, docs); err != nil {
		t.Fatalf("large batch upsert: %v", err)
	}

	// Spot check a few.
	for _, i := range []int{0, 123, 4999} {
		id := fmt.Sprintf("id%04d", i)
		hits, _, _ := e.Search(ctx, fmt.Sprintf("title %d", i), "", 1)
		if len(hits) != 1 || hits[0].ID != id {
			t.Fatalf("expected %s, got %+v", id, hits)
		}
	}
}

func TestConcurrentUpserts(t *testing.T) {
	e := newBatchTestEngine(t)
	ctx := t.Context()

	const workers = 20
	const perWorker = 100
	var wg sync.WaitGroup

	wg.Add(workers)
	for w := range workers {
		go func(w int) {
			defer wg.Done()
			for i := range perWorker {
				docID := fmt.Sprintf("w%02d_%03d", w, i)
				_ = e.Upsert(ctx, docID, map[string]string{
					testColumnTitle: "title " + docID,
					colBody:         "concurrent",
					testTag:         "c",
				})
			}
		}(w)
	}
	wg.Wait()

	// Ensure total documents match.
	rows, _, err := e.BatchList(ctx, "", nil, "", 10000)
	if err != nil {
		t.Fatalf("batchlist after concurrency: %v", err)
	}
	want := workers * perWorker
	if len(rows) != want {
		t.Fatalf("expected %d docs, got %d", want, len(rows))
	}
}

func TestBatchList_BasicAndEdgeCases(t *testing.T) {
	e := newBatchTestEngine(t)
	ctx := t.Context()

	// Insert 20 docs.
	docs := map[string]map[string]string{}
	for i := range 20 {
		docs[fmt.Sprintf("d%02d", i)] = map[string]string{
			testColumnTitle: fmt.Sprintf("t%d", i),
			colBody:         fmt.Sprintf("b%d", i),
			testTag:         fmt.Sprintf("tag%d", i%3),
		}
	}
	if err := e.BatchUpsert(ctx, docs); err != nil {
		t.Fatalf("setup batch: %v", err)
	}

	type testCase struct {
		name         string
		compareCol   string
		wantedCols   []string
		pageSize     int
		expectErr    bool
		expectRows   int
		expectTokens int
	}
	tests := []testCase{
		{
			name:         "default paging, all cols",
			pageSize:     7,
			expectRows:   20,
			expectTokens: 3,
		},
		{
			name:         "wantedCols subset",
			wantedCols:   []string{testColumnTitle},
			pageSize:     5,
			expectRows:   20,
			expectTokens: 4,
		},
		{
			name:         "compareColumn tag",
			compareCol:   testTag,
			pageSize:     10,
			expectRows:   20,
			expectTokens: 2,
		},
		{
			name:       "unknown compareColumn",
			compareCol: "doesnotexist",
			expectErr:  true,
		},
		{
			name:       "unknown wantedCol",
			wantedCols: []string{testColumnTitle, "nope"},
			expectErr:  true,
		},
		{
			name:         "pageSize 0 uses default",
			pageSize:     0,
			expectRows:   20,
			expectTokens: 1,
		},
		{
			name:         "pageSize negative uses default",
			pageSize:     -5,
			expectRows:   20,
			expectTokens: 1,
		},
		{
			name:         "pageSize > max capped",
			pageSize:     20000,
			expectRows:   20,
			expectTokens: 1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			token := ""
			total := 0
			pages := 0
			for {
				rows, next, err := e.BatchList(
					ctx,
					tc.compareCol,
					tc.wantedCols,
					token,
					tc.pageSize,
				)
				if tc.expectErr {
					if err == nil {
						t.Fatalf("expected error, got nil")
					}
					return
				}
				if err != nil {
					t.Fatalf("batchlist: %v", err)
				}
				total += len(rows)
				pages++
				if next == "" {
					break
				}
				token = next
			}
			if tc.expectRows > 0 && total != tc.expectRows {
				t.Fatalf("expected %d rows, got %d", tc.expectRows, total)
			}
			if tc.expectTokens > 0 && pages != tc.expectTokens {
				t.Fatalf("expected %d pages, got %d", tc.expectTokens, pages)
			}
		})
	}
}

func TestBatchList_PaginationAndTokenCorrectness(t *testing.T) {
	e := newBatchTestEngine(t)
	ctx := t.Context()

	// Insert 30 docs with random tag values.
	docs := map[string]map[string]string{}
	for i := range 30 {
		n, err := rand.Int(rand.Reader, big.NewInt(5))
		if err != nil {
			t.Fatalf("cannot generate rand number: %v", err)
		}
		docs[fmt.Sprintf("doc%02d", i)] = map[string]string{
			testColumnTitle: fmt.Sprintf("title%d", i),
			colBody:         fmt.Sprintf("body%d", i),
			testTag:         fmt.Sprintf("tag%d", n),
		}
	}
	if err := e.BatchUpsert(ctx, docs); err != nil {
		t.Fatalf("setup batch: %v", err)
	}

	// Page through all documents, ensure no duplicates.
	seen := map[string]bool{}
	token := ""
	for {
		rows, next, err := e.BatchList(ctx, testTag, []string{testColumnTitle, testTag}, token, 8)
		if err != nil {
			t.Fatalf("batchlist: %v", err)
		}
		for _, r := range rows {
			if seen[r.ID] {
				t.Fatalf("duplicate id %s across pages", r.ID)
			}
			seen[r.ID] = true
			if len(r.Values) != 2 {
				t.Fatalf("expected 2 values, got %+v", r.Values)
			}
		}
		if next == "" {
			break
		}
		token = next
	}
	if len(seen) != 30 {
		t.Fatalf("expected 30 docs, got %d", len(seen))
	}
}

func TestBatchList_TokenTamperingAndBoundary(t *testing.T) {
	e := newBatchTestEngine(t)
	ctx := t.Context()

	// Insert 5 docs.
	docs := map[string]map[string]string{}
	for i := range 5 {
		docs[fmt.Sprintf("x%d", i)] = map[string]string{
			testColumnTitle: fmt.Sprintf("t%d", i),
			colBody:         fmt.Sprintf("b%d", i),
			testTag:         testTag,
		}
	}
	if err := e.BatchUpsert(ctx, docs); err != nil {
		t.Fatalf("setup batch: %v", err)
	}

	t.Run("invalid base64 token", func(t *testing.T) {
		_, _, err := e.BatchList(ctx, "", nil, "!!!notbase64", 2)
		if err != nil {
			t.Fatalf("should ignore invalid token, got error: %v", err)
		}
	})

	t.Run("malformed json token", func(t *testing.T) {
		bad := base64.StdEncoding.EncodeToString([]byte("{notjson"))
		_, _, err := e.BatchList(ctx, "", nil, bad, 2)
		if err != nil {
			t.Fatalf("should ignore malformed token, got error: %v", err)
		}
	})

	t.Run("token for different compareColumn", func(t *testing.T) {
		// Get a token for compareColumn testTag.
		_, token, err := e.BatchList(ctx, testTag, nil, "", 2)
		if err != nil {
			t.Fatalf("batchlist: %v", err)
		}
		// Use it for default compareColumn.
		rows, _, err := e.BatchList(ctx, "", nil, token, 2)
		if err != nil {
			t.Fatalf("should not error, got: %v", err)
		}
		if len(rows) == 0 {
			t.Fatalf("should still return rows")
		}
	})
}

func TestBatchList_OrderingAndStability(t *testing.T) {
	e := newBatchTestEngine(t)
	ctx := t.Context()

	// Insert docs with duplicate compareColumn values.
	docs := map[string]map[string]string{
		"a": {testColumnTitle: testValueFoo, colBody: testBar, testTag: testDupColumnName},
		"b": {testColumnTitle: "baz", colBody: "qux", testTag: testDupColumnName},
		"c": {testColumnTitle: "quux", colBody: "corge", testTag: testDupColumnName},
		"d": {testColumnTitle: "grault", colBody: "garply", testTag: "unique"},
	}
	if err := e.BatchUpsert(ctx, docs); err != nil {
		t.Fatalf("setup batch: %v", err)
	}

	// Page with compareColumn testTag, pageSize 2.
	token := ""
	var ids []string
	for {
		rows, next, err := e.BatchList(ctx, testTag, []string{testColumnTitle}, token, 2)
		if err != nil {
			t.Fatalf("batchlist: %v", err)
		}
		for _, r := range rows {
			ids = append(ids, r.ID)
		}
		if next == "" {
			break
		}
		token = next
	}
	if len(ids) != 4 {
		t.Fatalf("expected 4 docs, got %d", len(ids))
	}
	// Must be ordered by tag then rowid - therefore all "dup" first.
	for i, id := range ids {
		if docs[id][testTag] == "unique" && i < 3 {
			t.Fatalf("unique tag appeared before all dup ones")
		}
	}
}

func TestBatchList_EmptyTable(t *testing.T) {
	e := newBatchTestEngine(t)
	ctx := t.Context()

	rows, next, err := e.BatchList(ctx, "", nil, "", 10)
	if err != nil {
		t.Fatalf("empty table: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("expected 0 rows, got %d", len(rows))
	}
	if next != "" {
		t.Fatalf("expected empty next token, got %q", next)
	}
}

func TestMemoryDBBasicCRUD(t *testing.T) {
	e := newMemoryEngine(t)
	ctx := t.Context()

	_ = e.Upsert(ctx, "m1", map[string]string{"c": testHello})
	hits, _, _ := e.Search(ctx, testHello, "", 5)
	if len(hits) != 1 || hits[0].ID != "m1" {
		t.Fatalf("memory db failed search, hits=%+v", hits)
	}
	_ = e.Delete(ctx, "m1")
	emp, _ := e.IsEmpty(ctx)
	if !emp {
		t.Fatalf("IsEmpty should be true after delete on memory db")
	}
}

func TestRaceDetectorSmoke(t *testing.T) {
	// This test does a very small concurrent workload so that `go test -race`
	// has a chance to observe data races on the mutex-protected code.
	e := newTestEngine(t)
	ctx := t.Context()

	var wg sync.WaitGroup
	for i := range 5 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id := fmt.Sprintf("r%d", i)
			for j := range 50 {
				_ = e.Upsert(ctx, id, map[string]string{
					testColumnTitle: fmt.Sprintf("t%d", j),
					colBody:         fmt.Sprintf("b%d", j),
				})
				if j%10 == 0 {
					_ = e.Delete(ctx, id)
				}
				_, _, _ = e.Search(ctx, "t", "", 3)
			}
		}(i)
	}
	wg.Wait()

	// Wait a moment so that WAL checkpoints can finish.
	time.Sleep(50 * time.Millisecond)
}

func TestSchemaPersistenceUnchanged(t *testing.T) {
	tmp := t.TempDir()
	cfg := Config{
		BaseDir:    tmp,
		DBFileName: testDBFileName,
		Table:      testDocsTable,
		Columns:    []Column{{Name: colBody}},
	}

	e1, _ := NewEngine(cfg)
	if err := e1.Upsert(t.Context(), testDoc1, map[string]string{colBody: testHello}); err != nil {
		t.Fatalf("insert v1: %v", err)
	}
	e1.Close()

	// Re-open with the *identical* configuration - row must still exist.
	e2, _ := NewEngine(cfg)
	defer e2.Close()

	hits, _, _ := e2.Search(t.Context(), testHello, "", 10)
	if len(hits) != 1 || hits[0].ID != testDoc1 {
		t.Fatalf("row vanished after reopen, hits=%+v", hits)
	}
}

func TestUnindexedColumnIsNotSearchable(t *testing.T) {
	e, err := NewEngine(Config{
		BaseDir:    t.TempDir(),
		DBFileName: testDBFileName,
		Table:      testDocsTable,
		Columns: []Column{
			{Name: testColumnTitle},
			{Name: testSecret, Unindexed: true},
		},
	})
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	defer e.Close()
	_ = e.Upsert(t.Context(), "d1", map[string]string{
		testColumnTitle: "public",
		testSecret:      "top-secret",
	})

	// "secret" must NOT be searchable.
	if hits, _, _ := e.Search(t.Context(), "top-secret", "", 5); len(hits) != 0 {
		t.Fatalf("unindexed column affected search: %+v", hits)
	}

	// But we can still list the value.
	rows, _, _ := e.BatchList(t.Context(), "", []string{testColumnTitle, testSecret}, "", 5)
	if len(rows) != 1 || rows[0].Values[testSecret] != "top-secret" {
		t.Fatalf("unindexed column missing from list: %+v", rows)
	}
}

func TestBatchUpsertAtomicity(t *testing.T) {
	e := newBatchTestEngine(t)
	ctx := t.Context()

	batch := map[string]map[string]string{
		"good": {testColumnTitle: "ok"},
		"":     {testColumnTitle: "bad"}, // Illegal id - should make the whole tx fail.
	}
	if err := e.BatchUpsert(ctx, batch); err == nil {
		t.Fatalf("want error for empty id, got nil")
	}

	// No partial commit must have happened.
	if hits, _, _ := e.Search(ctx, "ok", "", 10); len(hits) != 0 {
		t.Fatalf("partial commit occurred, hits=%+v", hits)
	}
}

func TestUpsertUnknownColumnIgnored(t *testing.T) {
	e := newTestEngine(t)

	if err := e.Upsert(t.Context(), "u1", map[string]string{
		testColumnTitle: "known",
		"unknown":       "ignored",
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	hits, _, _ := e.Search(t.Context(), "known", "", 5)
	if len(hits) != 1 {
		t.Fatalf("row missing after upsert with unknown column")
	}
}

func TestHelperQuoteAndPlaceholders(t *testing.T) {
	if want, got := `"foo""bar"`, quote(`foo"bar`); got != want {
		t.Fatalf("quote failed, want %q, got %q", want, got)
	}

	if s := paramPlaceholders(0); s != "" {
		t.Fatalf("paramPlaceholders(0) = %q, want empty", s)
	}
	if s := paramPlaceholders(3); s != ",?,?,?" {
		t.Fatalf("unexpected placeholders: %q", s)
	}
}

func TestSearchZeroHits(t *testing.T) {
	e := newTestEngine(t)
	ctx := t.Context()

	// Insert a document that does NOT contain the search term.
	err := e.Upsert(ctx, testDoc1, map[string]string{
		testColumnTitle: testHelloWorld,
		colBody:         "this is a test",
	})
	if err != nil {
		t.Fatalf("failed to insert doc: %v", err)
	}

	// Search for a term that does not exist.
	hits, token, err := e.Search(ctx, "no-match", "", 10)
	if err != nil {
		t.Fatalf("search error: %v", err)
	}
	if len(hits) != 0 {
		t.Fatalf("expected zero hits, got %d", len(hits))
	}
	if token != "" {
		t.Fatalf("expected empty continuation token, got %q", token)
	}
}

func TestSearchOnlySpecialChars(t *testing.T) {
	e := newTestEngine(t)
	ctx := t.Context()
	_ = e.Upsert(ctx, "s1", map[string]string{testColumnTitle: testValueFoo, colBody: ""})
	hits, _, _ := e.Search(ctx, "*", "", 10)
	// Should not match anything, but should not error.
	if len(hits) != 0 {
		t.Fatalf("expected 0 hits for special char query, got %d", len(hits))
	}
}

func TestBatchUpsertAllNilMaps(t *testing.T) {
	e := newTestEngine(t)
	ctx := t.Context()
	docs := map[string]map[string]string{
		"a": nil,
		"b": nil,
	}
	if err := e.BatchUpsert(ctx, docs); err != nil {
		t.Fatalf("batch upsert all nil: %v", err)
	}
}

func TestBatchListAllUnindexed(t *testing.T) {
	e, err := NewEngine(Config{
		BaseDir:    t.TempDir(),
		DBFileName: testDBFileName,
		Table:      testDocsTable,
		Columns: []Column{
			{Name: testColumnTitle, Unindexed: true},
			{Name: colBody, Unindexed: true},
		},
	})
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	defer e.Close()

	_ = e.Upsert(t.Context(), "d1", map[string]string{testColumnTitle: testValueFoo, colBody: testBar})
	rows, _, _ := e.BatchList(t.Context(), "", []string{testColumnTitle, colBody}, "", 5)
	if len(rows) != 1 || rows[0].Values[testColumnTitle] != testValueFoo {
		t.Fatalf("batchlist failed for all unindexed columns: %+v", rows)
	}
}

func TestAllColumnsUnindexed(t *testing.T) {
	e, err := NewEngine(Config{
		BaseDir:    t.TempDir(),
		DBFileName: testDBFileName,
		Table:      testDocsTable,
		Columns: []Column{
			{Name: testColumnTitle, Unindexed: true},
			{Name: colBody, Unindexed: true},
		},
	})
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	defer e.Close()
	_ = e.Upsert(t.Context(), "d1", map[string]string{testColumnTitle: testValueFoo, colBody: testBar})
	hits, _, _ := e.Search(t.Context(), testValueFoo, "", 5)
	if len(hits) != 0 {
		t.Fatalf("search should return 0 hits when all columns unindexed")
	}
}

func TestBatchUpsertDuplicateIDs(t *testing.T) {
	e := newTestEngine(t)
	ctx := t.Context()
	docs := map[string]map[string]string{
		testDupColumnName: {testColumnTitle: "first"},
	}
	docs[testDupColumnName] = map[string]string{testColumnTitle: testSecond}
	if err := e.BatchUpsert(ctx, docs); err != nil {
		t.Fatalf("batch upsert: %v", err)
	}
	hits, _, _ := e.Search(ctx, testSecond, "", 10)
	if len(hits) != 1 || hits[0].ID != testDupColumnName {
		t.Fatalf("expected only last value for dup id, hits=%+v", hits)
	}
}

func TestSpecialCharactersInQuery(t *testing.T) {
	e := newTestEngine(t)
	ctx := t.Context()
	_ = e.Upsert(ctx, "s1", map[string]string{testColumnTitle: "foo*bar", colBody: ""})
	hits, _, _ := e.Search(ctx, "foo*bar", "", 10)
	if len(hits) != 1 || hits[0].ID != "s1" {
		t.Fatalf("special char search failed, hits=%+v", hits)
	}
}

func TestEmptyStringValues(t *testing.T) {
	e := newTestEngine(t)
	ctx := t.Context()
	err := e.Upsert(ctx, "empty", map[string]string{testColumnTitle: "", colBody: ""})
	if err != nil {
		t.Fatalf("upsert empty values: %v", err)
	}
	hits, _, _ := e.Search(ctx, "", "", 10)
	// Should not match anything, but should not error.
	if len(hits) != 0 {
		t.Fatalf("expected 0 hits for empty search, got %d", len(hits))
	}
}

func TestUnicodeAndDiacritics(t *testing.T) {
	e := newTestEngine(t)
	ctx := t.Context()
	_ = e.Upsert(ctx, "u1", map[string]string{testColumnTitle: "café", colBody: ""})
	hits, _, _ := e.Search(ctx, "cafe", "", 10)
	if len(hits) != 1 || hits[0].ID != "u1" {
		t.Fatalf("diacritic-insensitive search failed, hits=%+v", hits)
	}
}

func TestVeryLongIDsAndValues(t *testing.T) {
	e := newTestEngine(t)
	ctx := t.Context()
	longID := strings.Repeat("x", 1000)
	longVal := strings.Repeat("foo ", 5000)
	t.Logf("Upserting doc with ID len=%d, val len=%d", len(longID), len(longVal))
	err := e.Upsert(ctx, longID, map[string]string{testColumnTitle: longVal, colBody: ""})
	if err != nil {
		t.Fatalf("upsert long id/val: %v", err)
	}
	t.Log("Upsert succeeded, searching for 'foo'")
	hits, next, err := e.Search(ctx, testValueFoo, "", 10)
	t.Logf("Search returned %d hits, next=%q, err=%v", len(hits), next, err)

	if len(hits) != 1 || hits[0].ID != longID {
		t.Fatalf("long id/val search failed, hits=%+v", hits)
	}
}

func TestBatchDelete(t *testing.T) {
	type testCase struct {
		name         string
		setupIDs     []string
		deleteIDs    []string
		expectRemain []string
		expectErr    bool
	}

	const maxVars = 999

	longID := strings.Repeat("x", 1000)
	ids999 := make([]string, maxVars)
	for i := range ids999 {
		ids999[i] = fmt.Sprintf("id%03d", i)
	}

	tests := []testCase{
		{
			name:         "empty input is no-op",
			setupIDs:     []string{"a", "b"},
			deleteIDs:    nil,
			expectRemain: []string{"a", "b"},
		},
		{
			name:         "delete single existing",
			setupIDs:     []string{"a", "b"},
			deleteIDs:    []string{"a"},
			expectRemain: []string{"b"},
		},
		{
			name:         "delete all existing",
			setupIDs:     []string{"a", "b"},
			deleteIDs:    []string{"a", "b"},
			expectRemain: nil,
		},
		{
			name:         "delete non-existent id",
			setupIDs:     []string{"a"},
			deleteIDs:    []string{testNotFound},
			expectRemain: []string{"a"},
		},
		{
			name:         "delete mix of existing and non-existent",
			setupIDs:     []string{"a", "b"},
			deleteIDs:    []string{"a", testNotFound},
			expectRemain: []string{"b"},
		},
		{
			name:         "delete duplicate ids",
			setupIDs:     []string{"a", "b"},
			deleteIDs:    []string{"a", "a", "b"},
			expectRemain: nil,
		},
		{
			name:         "delete with long id",
			setupIDs:     []string{longID, "short"},
			deleteIDs:    []string{longID},
			expectRemain: []string{"short"},
		},
		{
			name:         "delete maxVars (999) in one call",
			setupIDs:     ids999,
			deleteIDs:    ids999,
			expectRemain: nil,
		},
		{
			name:         "delete more than maxVars (1001) triggers batching",
			setupIDs:     append(ids999, "x1", "x2"),
			deleteIDs:    append(ids999, "x1", "x2"),
			expectRemain: nil,
		},
		{
			name:         "delete with nil engine (closed)",
			setupIDs:     []string{"a"},
			deleteIDs:    []string{"a"},
			expectRemain: nil,
			// We'll close engine before delete.
			expectErr: true,
		},
		{
			name:      "delete after schema change (table recreated)",
			setupIDs:  []string{"a"},
			deleteIDs: []string{"a"},
			// Table will be empty after schema change anyway.
			expectRemain: nil,
			expectErr:    false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := newTestEngine(t)
			ctx := t.Context()
			insertDocs(t, e, tc.setupIDs)

			// Special handling for schema change test.
			if tc.name == "delete after schema change (table recreated)" {
				// Change schema (add a column), which drops all rows.
				cfg := e.cfg
				cfg.Columns = append(cfg.Columns, Column{Name: "extra"})
				e.Close()
				e2, err := NewEngine(cfg)
				if err != nil {
					t.Fatalf("schema change: %v", err)
				}
				defer e2.Close()
				e = e2
			}

			// Special handling for closed engine.
			if tc.name == "delete with nil engine (closed)" {
				e.Close()
			}

			err := e.BatchDelete(ctx, tc.deleteIDs)
			if tc.expectErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			} else if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Check remaining docs.
			rows, _, err := e.BatchList(ctx, "", nil, "", 1000)
			if err != nil {
				t.Fatalf("batchlist: %v", err)
			}
			got := make(map[string]bool)
			for _, r := range rows {
				got[r.ID] = true
			}
			for _, want := range tc.expectRemain {
				if !got[want] {
					t.Errorf("expected to find %q, but missing", want)
				}
				delete(got, want)
			}
			for extra := range got {
				t.Errorf("unexpected extra doc: %q", extra)
			}
		})
	}
}

func TestBatchDelete_Atomicity(t *testing.T) {
	e := newTestEngine(t)
	ctx := t.Context()
	ids := []string{"a", "b", "c"}
	insertDocs(t, e, ids)

	// Try to delete with an empty string ID (should not error, but nothing deleted).
	err := e.BatchDelete(ctx, []string{"a", ""})
	if err != nil {
		t.Fatalf("unexpected error for empty id: %v", err)
	}
	// Only "a" should be deleted.
	rows, _, _ := e.BatchList(ctx, "", nil, "", 10)
	got := make([]string, 0, len(rows))
	for _, r := range rows {
		got = append(got, r.ID)
	}
	if len(got) != 2 || (got[0] != "b" && got[1] != "c") {
		t.Fatalf("expected b and c to remain, got %v", got)
	}
}

func TestBatchDelete_AllRows(t *testing.T) {
	e := newTestEngine(t)
	ctx := t.Context()
	ids := []string{"a", "b", "c"}
	insertDocs(t, e, ids)
	err := e.BatchDelete(ctx, ids)
	if err != nil {
		t.Fatalf("delete all: %v", err)
	}
	empty, _ := e.IsEmpty(ctx)
	if !empty {
		t.Fatalf("expected table to be empty after delete all")
	}
}

func TestBatchDelete_NonexistentIDs(t *testing.T) {
	e := newTestEngine(t)
	ctx := t.Context()
	insertDocs(t, e, []string{"a"})
	err := e.BatchDelete(ctx, []string{testNotFound})
	if err != nil {
		t.Fatalf("delete non-existent: %v", err)
	}
	rows, _, _ := e.BatchList(ctx, "", nil, "", 10)
	if len(rows) != 1 || rows[0].ID != "a" {
		t.Fatalf("expected 'a' to remain, got %+v", rows)
	}
}

func TestBatchDelete_SQLiteSpecialChars(t *testing.T) {
	e := newTestEngine(t)
	ctx := t.Context()
	specialID := `weird"id'with,commas`
	insertDocs(t, e, []string{specialID, "normal"})
	err := e.BatchDelete(ctx, []string{specialID})
	if err != nil {
		t.Fatalf("delete special char id: %v", err)
	}
	rows, _, _ := e.BatchList(ctx, "", nil, "", 10)
	if len(rows) != 1 || rows[0].ID != "normal" {
		t.Fatalf("expected only 'normal' to remain, got %+v", rows)
	}
}

// Helper to insert docs for deletion tests.
func insertDocs(t *testing.T, e *Engine, ids []string) {
	t.Helper()
	for _, id := range ids {
		if err := e.Upsert(t.Context(), id, map[string]string{testColumnTitle: id, colBody: "test"}); err != nil {
			t.Fatalf("insert %q: %v", id, err)
		}
	}
}

// newBatchTestEngine returns an engine with three columns, useful for batch tests.
func newBatchTestEngine(t *testing.T) *Engine {
	t.Helper()
	tmp := t.TempDir()
	e, err := NewEngine(Config{
		BaseDir:    tmp,
		DBFileName: testDBFileName,
		Table:      testDocsTable,
		Columns: []Column{
			{Name: testColumnTitle, Weight: 1},
			{Name: colBody, Weight: 2},
			{Name: testTag, Weight: 1},
		},
	})
	if err != nil {
		t.Fatalf("engine init: %v", err)
	}
	t.Cleanup(func() { _ = e.Close() }) // <- important on Windows
	return e
}

// newTestEngine returns an engine with the two canonical columns testColumnTitle and "body".
func newTestEngine(t *testing.T) *Engine {
	t.Helper()
	tmp := t.TempDir()
	e, err := NewEngine(Config{
		BaseDir:    tmp,
		DBFileName: testDBFileName,
		Table:      testDocsTable,
		Columns: []Column{
			{Name: testColumnTitle, Weight: 1},
			{Name: colBody, Weight: 5},
		},
	})
	if err != nil {
		t.Fatalf("engine init: %v", err)
	}
	t.Cleanup(func() { _ = e.Close() }) // <- important on Windows
	return e
}

// newMemoryEngine builds an in-memory database (MemoryDBBaseDir).
func newMemoryEngine(t *testing.T) *Engine {
	t.Helper()
	e, err := NewEngine(Config{
		BaseDir: MemoryDBBaseDir,
		Table:   "memdocs",
		Columns: []Column{
			{Name: "c"},
		},
	})
	if err != nil {
		t.Fatalf("mem engine init: %v", err)
	}
	t.Cleanup(func() { _ = e.Close() }) // <- important on Windows
	return e
}
