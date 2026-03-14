package version_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tetora/internal/version"
)

func setupDB(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	if err := version.InitDB(dbPath); err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	return dbPath
}

func TestInitDB(t *testing.T) {
	dbPath := setupDB(t)
	// Should be idempotent.
	if err := version.InitDB(dbPath); err != nil {
		t.Fatalf("second InitDB: %v", err)
	}
}

func TestSnapshotEntity(t *testing.T) {
	dbPath := setupDB(t)

	content := `{"key":"value"}`
	if err := version.SnapshotEntity(dbPath, "config", "config.json", content, "test", "initial"); err != nil {
		t.Fatalf("SnapshotEntity: %v", err)
	}

	versions, err := version.QueryVersions(dbPath, "config", "config.json", 10)
	if err != nil {
		t.Fatalf("QueryVersions: %v", err)
	}
	if len(versions) != 1 {
		t.Fatalf("expected 1 version, got %d", len(versions))
	}
	if versions[0].EntityType != "config" {
		t.Errorf("entityType: got %q, want %q", versions[0].EntityType, "config")
	}
	if versions[0].EntityName != "config.json" {
		t.Errorf("entityName: got %q, want %q", versions[0].EntityName, "config.json")
	}
	if versions[0].ChangedBy != "test" {
		t.Errorf("changedBy: got %q, want %q", versions[0].ChangedBy, "test")
	}
	if versions[0].DiffSummary != "initial version" {
		t.Errorf("diffSummary: got %q, want %q", versions[0].DiffSummary, "initial version")
	}
}

func TestSnapshotEntitySkipsDuplicateContent(t *testing.T) {
	dbPath := setupDB(t)

	content := `{"key":"value"}`
	version.SnapshotEntity(dbPath, "config", "config.json", content, "test", "first")
	version.SnapshotEntity(dbPath, "config", "config.json", content, "test", "second")

	versions, _ := version.QueryVersions(dbPath, "config", "config.json", 10)
	if len(versions) != 1 {
		t.Errorf("expected 1 version (skip duplicate), got %d", len(versions))
	}
}

func TestSnapshotEntityRecordsDiff(t *testing.T) {
	dbPath := setupDB(t)

	v1 := `{"key":"old","extra":"val"}`
	v2 := `{"key":"new","added":"field"}`

	version.SnapshotEntity(dbPath, "config", "config.json", v1, "test", "v1")
	version.SnapshotEntity(dbPath, "config", "config.json", v2, "test", "v2")

	versions, _ := version.QueryVersions(dbPath, "config", "config.json", 10)
	if len(versions) != 2 {
		t.Fatalf("expected 2 versions, got %d", len(versions))
	}

	// Most recent first.
	diff := versions[0].DiffSummary
	if diff == "" || diff == "initial version" || diff == "no changes" {
		t.Errorf("expected meaningful diff, got %q", diff)
	}
	if !strings.Contains(diff, "added") && !strings.Contains(diff, "+") {
		t.Errorf("diff should mention added field: %q", diff)
	}
}

func TestQueryLatest(t *testing.T) {
	dbPath := setupDB(t)

	version.SnapshotEntity(dbPath, "config", "config.json", `{"v":1}`, "test", "v1")
	version.SnapshotEntity(dbPath, "config", "config.json", `{"v":2}`, "test", "v2")

	latest, err := version.QueryLatest(dbPath, "config", "config.json")
	if err != nil {
		t.Fatalf("QueryLatest: %v", err)
	}
	if latest == nil {
		t.Fatal("expected non-nil version")
	}
	if latest.ContentJSON != `{"v":2}` {
		t.Errorf("content: got %q, want %q", latest.ContentJSON, `{"v":2}`)
	}
}

func TestQueryByID(t *testing.T) {
	dbPath := setupDB(t)

	version.SnapshotEntity(dbPath, "config", "config.json", `{"x":1}`, "test", "test-reason")

	versions, _ := version.QueryVersions(dbPath, "config", "config.json", 10)
	if len(versions) == 0 {
		t.Fatal("no versions")
	}

	ver, err := version.QueryByID(dbPath, versions[0].VersionID)
	if err != nil {
		t.Fatalf("QueryByID: %v", err)
	}
	if ver.ContentJSON != `{"x":1}` {
		t.Errorf("content mismatch")
	}
	if ver.Reason != "test-reason" {
		t.Errorf("reason: got %q, want %q", ver.Reason, "test-reason")
	}
}

func TestQueryByIDPrefixMatch(t *testing.T) {
	dbPath := setupDB(t)

	version.SnapshotEntity(dbPath, "config", "config.json", `{"p":"prefix"}`, "test", "")

	versions, _ := version.QueryVersions(dbPath, "config", "config.json", 10)
	if len(versions) == 0 {
		t.Fatal("no versions")
	}

	// Use first 3 characters as prefix.
	prefix := versions[0].VersionID[:3]
	ver, err := version.QueryByID(dbPath, prefix)
	if err != nil {
		t.Fatalf("QueryByID prefix: %v", err)
	}
	if ver.VersionID != versions[0].VersionID {
		t.Errorf("prefix mismatch: got %q, want %q", ver.VersionID, versions[0].VersionID)
	}
}

func TestQueryByIDNotFound(t *testing.T) {
	dbPath := setupDB(t)

	_, err := version.QueryByID(dbPath, "nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent version")
	}
}

func TestSnapshotConfig(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	version.InitDB(dbPath)

	configPath := filepath.Join(dir, "config.json")
	os.WriteFile(configPath, []byte(`{"listenAddr":"127.0.0.1:7777"}`), 0o644)

	if err := version.SnapshotConfig(dbPath, configPath, "test", "initial"); err != nil {
		t.Fatalf("SnapshotConfig: %v", err)
	}

	versions, _ := version.QueryVersions(dbPath, "config", "config.json", 10)
	if len(versions) != 1 {
		t.Errorf("expected 1 version, got %d", len(versions))
	}
}

func TestSnapshotWorkflow(t *testing.T) {
	dbPath := setupDB(t)

	wf := `{"name":"test-wf","steps":[{"id":"s1","prompt":"hello"}]}`
	if err := version.SnapshotWorkflow(dbPath, "test-wf", wf, "cli", "created"); err != nil {
		t.Fatalf("SnapshotWorkflow: %v", err)
	}

	versions, _ := version.QueryVersions(dbPath, "workflow", "test-wf", 10)
	if len(versions) != 1 {
		t.Errorf("expected 1 version, got %d", len(versions))
	}
	if versions[0].EntityName != "test-wf" {
		t.Errorf("entityName: got %q", versions[0].EntityName)
	}
}

func TestSnapshotPrompt(t *testing.T) {
	dbPath := setupDB(t)

	if err := version.SnapshotPrompt(dbPath, "greeting", "Hello {{name}}", "cli", "new prompt"); err != nil {
		t.Fatalf("SnapshotPrompt: %v", err)
	}

	versions, _ := version.QueryVersions(dbPath, "prompt", "greeting", 10)
	if len(versions) != 1 {
		t.Errorf("expected 1 version, got %d", len(versions))
	}
}

func TestRestoreConfig(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	version.InitDB(dbPath)
	configPath := filepath.Join(dir, "config.json")

	// Write initial config.
	v1 := `{"listenAddr":"127.0.0.1:7777","apiToken":"abc"}`
	os.WriteFile(configPath, []byte(v1), 0o644)
	version.SnapshotConfig(dbPath, configPath, "test", "v1")

	// Get v1 version ID.
	versions, _ := version.QueryVersions(dbPath, "config", "config.json", 10)
	v1ID := versions[0].VersionID

	// Modify config.
	v2 := `{"listenAddr":"0.0.0.0:8888","apiToken":"xyz"}`
	os.WriteFile(configPath, []byte(v2), 0o644)
	version.SnapshotConfig(dbPath, configPath, "test", "v2")

	// Restore to v1.
	prev, err := version.RestoreConfig(dbPath, configPath, v1ID)
	if err != nil {
		t.Fatalf("RestoreConfig: %v", err)
	}
	if prev != v2 {
		t.Errorf("previous content mismatch")
	}

	// Check file content was restored.
	data, _ := os.ReadFile(configPath)
	if string(data) != v1 {
		t.Errorf("restored config mismatch: got %q, want %q", string(data), v1)
	}
}

func TestRestoreConfigInvalidType(t *testing.T) {
	dbPath := setupDB(t)

	version.SnapshotEntity(dbPath, "workflow", "my-wf", `{"name":"my-wf"}`, "test", "")
	versions, _ := version.QueryVersions(dbPath, "workflow", "my-wf", 10)
	vid := versions[0].VersionID

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	os.WriteFile(configPath, []byte(`{}`), 0o644)

	_, err := version.RestoreConfig(dbPath, configPath, vid)
	if err == nil {
		t.Error("expected error for wrong entity type")
	}
	if !strings.Contains(err.Error(), "not a config") {
		t.Errorf("error should mention type mismatch: %v", err)
	}
}

func TestComputeJSONDiff(t *testing.T) {
	tests := []struct {
		name     string
		old      string
		new      string
		contains []string
	}{
		{
			name:     "no changes",
			old:      `{"a":1}`,
			new:      `{"a":1}`,
			contains: []string{"no changes"},
		},
		{
			name:     "added field",
			old:      `{"a":1}`,
			new:      `{"a":1,"b":2}`,
			contains: []string{"+", "b"},
		},
		{
			name:     "removed field",
			old:      `{"a":1,"b":2}`,
			new:      `{"a":1}`,
			contains: []string{"-", "b"},
		},
		{
			name:     "changed field",
			old:      `{"a":1}`,
			new:      `{"a":2}`,
			contains: []string{"~", "a"},
		},
		{
			name:     "nested change",
			old:      `{"a":{"x":1}}`,
			new:      `{"a":{"x":2}}`,
			contains: []string{"a.x"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := version.ComputeJSONDiff(tt.old, tt.new)
			for _, c := range tt.contains {
				if !strings.Contains(result, c) {
					t.Errorf("diff %q should contain %q", result, c)
				}
			}
		})
	}
}

func TestComputeTextDiff(t *testing.T) {
	old := "line1\nline2\nline3"
	new := "line1\nmodified\nline3\nline4"

	diff := version.ComputeTextDiff(old, new)
	if !strings.Contains(diff, "+") {
		t.Errorf("text diff should show additions: %q", diff)
	}
	if !strings.Contains(diff, "-") {
		t.Errorf("text diff should show removals: %q", diff)
	}
}

func TestComputeTextDiffNoChanges(t *testing.T) {
	text := "hello\nworld"
	diff := version.ComputeTextDiff(text, text)
	if diff != "no changes" {
		t.Errorf("expected 'no changes', got %q", diff)
	}
}

func TestDiffDetail(t *testing.T) {
	dbPath := setupDB(t)

	version.SnapshotEntity(dbPath, "config", "config.json", `{"a":1,"b":"old"}`, "test", "v1")
	version.SnapshotEntity(dbPath, "config", "config.json", `{"a":1,"b":"new","c":true}`, "test", "v2")

	versions, _ := version.QueryVersions(dbPath, "config", "config.json", 10)
	if len(versions) < 2 {
		t.Fatal("need 2 versions")
	}

	result, err := version.DiffDetail(dbPath, versions[1].VersionID, versions[0].VersionID)
	if err != nil {
		t.Fatalf("DiffDetail: %v", err)
	}

	changes, ok := result["changes"].([]map[string]any)
	if !ok {
		t.Fatal("expected changes array")
	}
	if len(changes) == 0 {
		t.Error("expected some changes")
	}

	foundChanged := false
	foundAdded := false
	for _, ch := range changes {
		if ch["field"] == "b" && ch["type"] == "changed" {
			foundChanged = true
		}
		if ch["field"] == "c" && ch["type"] == "added" {
			foundAdded = true
		}
	}
	if !foundChanged {
		t.Error("expected 'b' to be marked as changed")
	}
	if !foundAdded {
		t.Error("expected 'c' to be marked as added")
	}
}

func TestPrune(t *testing.T) {
	dbPath := setupDB(t)

	// Create 10 versions.
	for i := 0; i < 10; i++ {
		content := `{"v":` + strings.Repeat("x", i+1) + `}`
		version.SnapshotEntity(dbPath, "config", "config.json", content, "test", "")
	}

	// Prune to keep only 3.
	version.Prune(dbPath, "config", "config.json", 3)

	versions, _ := version.QueryVersions(dbPath, "config", "config.json", 50)
	if len(versions) != 3 {
		t.Errorf("expected 3 versions after prune, got %d", len(versions))
	}
}

func TestQueryAllEntities(t *testing.T) {
	dbPath := setupDB(t)

	version.SnapshotEntity(dbPath, "config", "config.json", `{"a":1}`, "test", "")
	version.SnapshotEntity(dbPath, "workflow", "deploy", `{"name":"deploy"}`, "test", "")
	version.SnapshotEntity(dbPath, "prompt", "greeting", "Hello!", "test", "")

	entities, err := version.QueryAllEntities(dbPath)
	if err != nil {
		t.Fatalf("QueryAllEntities: %v", err)
	}
	if len(entities) != 3 {
		t.Errorf("expected 3 entities, got %d", len(entities))
	}

	types := make(map[string]bool)
	for _, e := range entities {
		types[e.EntityType] = true
	}
	for _, expected := range []string{"config", "workflow", "prompt"} {
		if !types[expected] {
			t.Errorf("missing entity type %q", expected)
		}
	}
}

func TestGetNestedValue(t *testing.T) {
	m := map[string]any{
		"a": map[string]any{
			"b": map[string]any{
				"c": "deep",
			},
		},
		"top": "level",
	}

	if v := version.GetNestedValue(m, "top"); v != "level" {
		t.Errorf("got %v, want 'level'", v)
	}
	if v := version.GetNestedValue(m, "a.b.c"); v != "deep" {
		t.Errorf("got %v, want 'deep'", v)
	}
	if v := version.GetNestedValue(m, "missing"); v != nil {
		t.Errorf("got %v, want nil", v)
	}
	if v := version.GetNestedValue(m, "a.missing"); v != nil {
		t.Errorf("got %v, want nil", v)
	}
}

func TestNewID(t *testing.T) {
	ids := make(map[string]bool)
	for i := 0; i < 100; i++ {
		id := version.NewID()
		if !strings.HasPrefix(id, "v-") {
			t.Errorf("version ID should start with 'v-': %q", id)
		}
		if ids[id] {
			t.Errorf("duplicate version ID: %q", id)
		}
		ids[id] = true
	}
}

func TestSnapshotEmptyDB(t *testing.T) {
	if err := version.SnapshotConfig("", "/nonexistent/config.json", "test", ""); err != nil {
		t.Errorf("expected nil error for empty dbPath, got %v", err)
	}
	if err := version.SnapshotWorkflow("", "wf", "{}", "test", ""); err != nil {
		t.Errorf("expected nil error for empty dbPath, got %v", err)
	}
	if err := version.SnapshotPrompt("", "prompt", "hello", "test", ""); err != nil {
		t.Errorf("expected nil error for empty dbPath, got %v", err)
	}
}

func TestFlattenDiffNestedAdd(t *testing.T) {
	old := map[string]any{"a": map[string]any{"x": 1}}
	new := map[string]any{"a": map[string]any{"x": 1, "y": 2}}

	var added, removed, changed []string
	version.FlattenDiff("", old, new, &added, &removed, &changed)

	if len(added) != 1 || added[0] != "a.y" {
		t.Errorf("expected added=['a.y'], got %v", added)
	}
	if len(removed) != 0 {
		t.Errorf("expected no removals, got %v", removed)
	}
	if len(changed) != 0 {
		t.Errorf("expected no changes, got %v", changed)
	}
}

func TestFlattenDiffArrayChange(t *testing.T) {
	old := map[string]any{"tags": []any{"a", "b"}}
	new := map[string]any{"tags": []any{"a", "c"}}

	var added, removed, changed []string
	version.FlattenDiff("", old, new, &added, &removed, &changed)

	if len(changed) != 1 || changed[0] != "tags" {
		t.Errorf("expected changed=['tags'], got %v", changed)
	}
}

func TestMultipleEntitiesIsolation(t *testing.T) {
	dbPath := setupDB(t)

	version.SnapshotEntity(dbPath, "config", "config.json", `{"a":1}`, "test", "")
	version.SnapshotEntity(dbPath, "workflow", "deploy", `{"name":"deploy"}`, "test", "")

	configVersions, _ := version.QueryVersions(dbPath, "config", "config.json", 10)
	workflowVersions, _ := version.QueryVersions(dbPath, "workflow", "deploy", 10)

	if len(configVersions) != 1 {
		t.Errorf("expected 1 config version, got %d", len(configVersions))
	}
	if len(workflowVersions) != 1 {
		t.Errorf("expected 1 workflow version, got %d", len(workflowVersions))
	}
}

func TestDiffDetailNotFound(t *testing.T) {
	dbPath := setupDB(t)

	_, err := version.DiffDetail(dbPath, "v-abc", "v-def")
	if err == nil {
		t.Error("expected error for nonexistent versions")
	}
}

func TestComputeJSONDiffManyFields(t *testing.T) {
	// Test with > 5 changes to trigger count mode.
	old := `{"a":1,"b":2,"c":3,"d":4,"e":5,"f":6}`
	new := `{"a":10,"b":20,"c":30,"d":40,"e":50,"f":60}`

	diff := version.ComputeJSONDiff(old, new)
	if !strings.Contains(diff, "6 fields changed") {
		t.Errorf("expected count summary for many changes: %q", diff)
	}
}

func TestComputeJSONDiffInvalidJSON(t *testing.T) {
	diff := version.ComputeJSONDiff("not json", `{"a":1}`)
	if !strings.Contains(diff, "not valid JSON") {
		t.Errorf("expected invalid JSON message: %q", diff)
	}
}

func TestVersionIDFormat(t *testing.T) {
	id := version.NewID()
	if !strings.HasPrefix(id, "v-") {
		t.Errorf("ID should start with 'v-': %q", id)
	}
	if len(id) != 10 { // "v-" + 8 hex chars
		t.Errorf("ID length should be 10, got %d: %q", len(id), id)
	}
}

func TestQueryVersionsLimit(t *testing.T) {
	dbPath := setupDB(t)

	// Create 5 versions.
	for i := 0; i < 5; i++ {
		version.SnapshotEntity(dbPath, "config", "config.json",
			`{"v":`+json.Number(strings.Repeat("1", i+1)).String()+`}`, "test", "")
	}

	// Query with limit 2.
	versions, err := version.QueryVersions(dbPath, "config", "config.json", 2)
	if err != nil {
		t.Fatalf("QueryVersions: %v", err)
	}
	if len(versions) != 2 {
		t.Errorf("expected 2 versions with limit, got %d", len(versions))
	}
}
