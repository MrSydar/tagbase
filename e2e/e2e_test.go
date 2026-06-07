package e2e_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const storageURL = "http://localhost:8080"

var httpClient = &http.Client{Timeout: 30 * time.Second}

type tagQueryReq struct {
	Tags  map[string]bool `json:"tags,omitempty"`
	Limit int             `json:"limit"`
}

type tagQueryResp struct {
	Objects    []objMeta `json:"objects"`
	HasMore    bool      `json:"has_more"`
	NextCursor string    `json:"next_cursor,omitempty"`
}

type objMeta struct {
	ID          string    `json:"id"`
	Collection  string    `json:"collection"`
	DataType    string    `json:"data_type"`
	Date        time.Time `json:"date"`
	SizeBytes   int64     `json:"size_bytes"`
	ContentHash string    `json:"content_hash"`
}

func TestEndToEnd(t *testing.T) {
	coll := "e2e_test_scenario"

	// 1. Create collection
	createBody := []byte(`{"name":"` + coll + `","data_type":"txt"}`)
	resp, err := http.Post(storageURL+"/v1/collections", "application/json", bytes.NewReader(createBody))
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	resp.Body.Close()

	defer func() {
		req, _ := http.NewRequest("DELETE", storageURL+"/v1/collections/"+coll, nil)
		r, _ := httpClient.Do(req)
		if r != nil {
			r.Body.Close()
		}
	}()

	// 2. Upload first object
	payload1 := []byte("a golang job description")
	obj1 := uploadObject(t, coll, "txt", payload1)

	// 3. Query without tags — should return obj1
	result := queryObjects(t, coll, tagQueryReq{Limit: 10})
	require.Len(t, result.Objects, 1)
	assert.Equal(t, obj1.ID, result.Objects[0].ID)

	// 4. Query with "golang" — should return obj1
	result = queryObjects(t, coll, tagQueryReq{Tags: map[string]bool{"golang": true}, Limit: 10})
	require.Len(t, result.Objects, 1)
	assert.Equal(t, obj1.ID, result.Objects[0].ID)

	// 5. Query with "java" — should return nothing
	result = queryObjects(t, coll, tagQueryReq{Tags: map[string]bool{"java": true}, Limit: 10})
	assert.Empty(t, result.Objects)

	// 6. Upload second object
	payload2 := []byte("a golang and java job description")
	obj2 := uploadObject(t, coll, "txt", payload2)

	// 7. Query no tags — both present
	result = queryObjects(t, coll, tagQueryReq{Limit: 10})
	ids := extractIDs(result.Objects)
	assert.Len(t, result.Objects, 2)
	assert.Contains(t, ids, obj1.ID)
	assert.Contains(t, ids, obj2.ID)

	// 8. Query "golang" — both present
	result = queryObjects(t, coll, tagQueryReq{Tags: map[string]bool{"golang": true}, Limit: 10})
	ids = extractIDs(result.Objects)
	assert.Len(t, result.Objects, 2)
	assert.Contains(t, ids, obj1.ID)
	assert.Contains(t, ids, obj2.ID)

	// 9. Query "java" — only obj2 present
	result = queryObjects(t, coll, tagQueryReq{Tags: map[string]bool{"java": true}, Limit: 10})
	ids = extractIDs(result.Objects)
	assert.Len(t, result.Objects, 1)
	assert.Contains(t, ids, obj2.ID)
	assert.NotContains(t, ids, obj1.ID)

	// 10. Query "c++" — nothing present
	result = queryObjects(t, coll, tagQueryReq{Tags: map[string]bool{"c++": true}, Limit: 10})
	assert.Empty(t, result.Objects)
}

func uploadObject(t *testing.T, collection, dataType string, data []byte) objMeta {
	t.Helper()
	url := fmt.Sprintf("%s/v1/collections/%s/objects?data_type=%s", storageURL, collection, dataType)
	req, err := http.NewRequest("POST", url, bytes.NewReader(data))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/octet-stream")
	resp, err := httpClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	require.Equal(t, http.StatusCreated, resp.StatusCode, "upload failed: %s", string(b))
	var obj objMeta
	require.NoError(t, json.Unmarshal(b, &obj))
	return obj
}

func queryObjects(t *testing.T, collection string, req tagQueryReq) tagQueryResp {
	t.Helper()
	url := fmt.Sprintf("%s/v1/collections/%s/objects/query", storageURL, collection)
	body, _ := json.Marshal(req)
	resp, err := http.Post(url, "application/json", bytes.NewReader(body))
	require.NoError(t, err)
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	require.Equal(t, http.StatusOK, resp.StatusCode, "query failed: %s", string(b))
	var result tagQueryResp
	require.NoError(t, json.Unmarshal(b, &result))
	return result
}

func extractIDs(objs []objMeta) []string {
	ids := make([]string, len(objs))
	for i, o := range objs {
		ids[i] = o.ID
	}
	return ids
}
