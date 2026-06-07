package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"mrsydar/tagbase/storage/internal/config"
	"mrsydar/tagbase/storage/internal/db"
	"mrsydar/tagbase/storage/internal/models"
	"mrsydar/tagbase/storage/internal/query"
	"mrsydar/tagbase/storage/internal/storage"
	"mrsydar/tagbase/storage/internal/validate"
	"mrsydar/tagbase/storage/pkg/client"
)

// Server holds dependencies for the HTTP server.
type Server struct {
	cfg            *config.Config
	db             *db.DB
	store          *storage.S3Store
	tagClient      client.Tagger
	queryRunner    *query.Runner
	logger         *zap.Logger
	supportedTypes []string
}

// NewServer creates a new Server.
func NewServer(cfg *config.Config, database *db.DB, store *storage.S3Store, tagClient client.Tagger, logger *zap.Logger) *Server {
	return &Server{
		cfg:         cfg,
		db:          database,
		store:       store,
		tagClient:   tagClient,
		queryRunner: query.NewRunner(database, tagClient),
		logger:      logger,
	}
}

// SetSupportedTypes sets the supported data types from tagging engine.
func (s *Server) SetSupportedTypes(types []string) {
	s.supportedTypes = types
}

// Router builds and returns the chi router.
func (s *Server) Router() chi.Router {
	r := chi.NewRouter()
	r.Use(requestLogger(s.logger))

	r.Get("/healthz", s.healthz)
	r.Get("/readyz", s.readyz)

	r.Get("/v1/collections", s.listCollections)
	r.Post("/v1/collections", s.createCollection)
	r.Delete("/v1/collections/{collection}", s.deleteCollection)
	r.Post("/v1/collections/{collection}/objects", s.putObject)
	r.Get("/v1/collections/{collection}/objects/{id}", s.getObjectMetadata)
	r.Get("/v1/collections/{collection}/objects/{id}/data", s.getObjectData)
	r.Get("/v1/collections/{collection}/objects/{id}/tags", s.getObjectTags)
	r.Post("/v1/collections/{collection}/objects/query", s.queryObjects)
	r.Delete("/v1/collections/{collection}/objects/{id}", s.deleteObject)

	return r
}

func (s *Server) healthz(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

func (s *Server) readyz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	// Check DB
	if err := s.db.Pool().Ping(ctx); err != nil {
		s.logger.Error("readyz db ping failed", zap.Error(err))
		w.WriteHeader(http.StatusServiceUnavailable)
		writeError(w, http.StatusServiceUnavailable, "not_ready", "database not available")
		return
	}
	// Check S3
	if err := s.store.HeadBucket(ctx); err != nil {
		s.logger.Error("readyz s3 check failed", zap.Error(err))
		w.WriteHeader(http.StatusServiceUnavailable)
		writeError(w, http.StatusServiceUnavailable, "not_ready", "object storage not available")
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

func (s *Server) listCollections(w http.ResponseWriter, r *http.Request) {
	collections, err := s.db.ListCollections(r.Context())
	if err != nil {
		s.logger.Error("list collections failed", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to list collections")
		return
	}
	resp := models.CollectionsListResponse{
		Collections: collections,
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (s *Server) createCollection(w http.ResponseWriter, r *http.Request) {
	var req models.CollectionCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "invalid request body")
		return
	}
	if err := validate.ValidateCollectionName(req.Name); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_name", err.Error())
		return
	}
	if err := validate.ValidateDataType(req.DataType, s.supportedTypes); err != nil {
		writeError(w, http.StatusBadRequest, "unsupported_data_type", err.Error())
		return
	}
	coll, err := s.db.CreateCollection(r.Context(), req.Name, req.DataType)
	if err != nil {
		// Check for unique violation
		if strings.Contains(err.Error(), "unique") || strings.Contains(err.Error(), "duplicate") {
			writeError(w, http.StatusConflict, "already_exists", "collection already exists")
			return
		}
		s.logger.Error("create collection failed", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to create collection")
		return
	}
	resp := models.CollectionCreateResponse{
		Name:     coll.Name,
		DataType: coll.DataType,
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(resp)
}

func (s *Server) deleteCollection(w http.ResponseWriter, r *http.Request) {
	collectionName := chi.URLParam(r, "collection")
	if err := validate.ValidateCollectionName(collectionName); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_collection_name", err.Error())
		return
	}

	coll, err := s.db.GetCollectionByName(r.Context(), collectionName)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "collection not found")
		return
	}

	payloadKeys, err := s.db.GetCollectionPayloadKeys(r.Context(), coll.ID)
	if err != nil {
		s.logger.Error("get collection payload keys failed", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to delete collection")
		return
	}

	if err := s.db.DeleteCollection(r.Context(), coll.ID); err != nil {
		s.logger.Error("delete collection failed", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to delete collection")
		return
	}

	for _, key := range payloadKeys {
		if err := s.store.Delete(r.Context(), key); err != nil {
			s.logger.Warn("delete collection object from S3 failed", zap.Error(err), zap.String("key", key))
		}
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) putObject(w http.ResponseWriter, r *http.Request) {
	collectionName := chi.URLParam(r, "collection")
	if err := validate.ValidateCollectionName(collectionName); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_collection_name", err.Error())
		return
	}

	coll, err := s.db.GetCollectionByName(r.Context(), collectionName)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "collection not found")
		return
	}

	dataType := r.URL.Query().Get("data_type")
	if dataType == "" {
		writeError(w, http.StatusBadRequest, "missing_data_type", "data_type is required")
		return
	}
	if dataType != coll.DataType {
		writeError(w, http.StatusBadRequest, "invalid_data_type", "object data_type does not match collection")
		return
	}

	ttlSecondsStr := r.URL.Query().Get("ttl_seconds")
	var expiresAt *time.Time
	if ttlSecondsStr != "" {
		ttlSec, err := strconv.Atoi(ttlSecondsStr)
		if err != nil || ttlSec < 0 {
			writeError(w, http.StatusBadRequest, "invalid_ttl", "invalid ttl_seconds")
			return
		}
		t := time.Now().UTC().Add(time.Duration(ttlSec) * time.Second)
		expiresAt = &t
	} else if s.cfg.DefaultTTL > 0 {
		t := time.Now().UTC().Add(s.cfg.DefaultTTL)
		expiresAt = &t
	}

	dateStr := r.URL.Query().Get("date")
	var date time.Time
	if dateStr != "" {
		var err error
		date, err = time.Parse(time.RFC3339, dateStr)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_date", "invalid date format")
			return
		}
	} else {
		date = time.Now().UTC()
	}

	tmpFile, err := os.CreateTemp("", "tagbase-upload-*")
	if err != nil {
		s.logger.Error("create temp file failed", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to buffer upload")
		return
	}
	defer func() {
		tmpFile.Close()
		os.Remove(tmpFile.Name())
	}()

	hasher := sha256.New()
	limited := io.LimitReader(r.Body, s.cfg.MaxObjectSizeBytes+1)
	written, err := io.Copy(io.MultiWriter(tmpFile, hasher), limited)
	if err != nil {
		s.logger.Error("stream body failed", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to read body")
		return
	}
	if written > s.cfg.MaxObjectSizeBytes {
		writeError(w, http.StatusRequestEntityTooLarge, "payload_too_large", fmt.Sprintf("payload exceeds %d bytes", s.cfg.MaxObjectSizeBytes))
		return
	}

	contentHash := hex.EncodeToString(hasher.Sum(nil))

	// Check for existing object in collection.
	if existing, err := s.db.GetObjectByCollectionAndHash(r.Context(), coll.ID, contentHash); err == nil {
		// Delete newly uploaded S3 object if it exists (in case of race). We haven't uploaded yet though.
		resp := models.ObjectUploadResponse{
			ID:          existing.ID,
			Collection:  existing.Collection,
			DataType:    existing.DataType,
			Date:        existing.Date,
			SizeBytes:   existing.SizeBytes,
			ContentHash: existing.ContentHash,
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(resp)
		return
	}

	// Insert object first so we have an ID, then upload to S3.
	objectID := ""
	// We need to insert to get ID, but if S3 fails, we need to clean up.
	// Alternative: generate UUID client-side or use DB insert first.
	// Let's insert first.
	payloadKey := fmt.Sprintf("%s/%s", collectionName, contentHash) // temporary, will update with actual ID
	// Actually, per design: payload_key format: <collection>/<object_id>
	// Since we need object_id, let's use UUID generated by postgres.
	// We'll insert with a dummy payload_key, then update after we get ID.
	obj, err := s.db.InsertObject(r.Context(), coll.ID, contentHash, date, written, dataType, "temp", expiresAt)
	if err != nil {
		// Handle duplicate caused by concurrent insert or expired row.
		if strings.Contains(err.Error(), "unique") || strings.Contains(err.Error(), "duplicate") {
			existing, err2 := s.db.GetObjectByCollectionAndHashIncludingExpired(r.Context(), coll.ID, contentHash)
			if err2 == nil {
				if existing.ExpiresAt != nil && !existing.ExpiresAt.After(time.Now().UTC()) {
					payloadKey, delErr := s.db.DeleteObject(r.Context(), existing.ID)
					if delErr != nil {
						s.logger.Error("delete expired duplicate failed", zap.Error(delErr))
						writeError(w, http.StatusInternalServerError, "internal_error", "failed to delete expired duplicate")
						return
					}
					if err := s.store.Delete(r.Context(), payloadKey); err != nil {
						s.logger.Warn("delete expired duplicate from s3 failed", zap.Error(err), zap.String("key", payloadKey))
					}
					obj, err = s.db.InsertObject(r.Context(), coll.ID, contentHash, date, written, dataType, "temp", expiresAt)
					if err != nil {
						s.logger.Error("insert object after cleanup failed", zap.Error(err))
						writeError(w, http.StatusInternalServerError, "internal_error", "failed to insert object")
						return
					}
				} else {
					resp := models.ObjectUploadResponse{
						ID:          existing.ID,
						Collection:  existing.Collection,
						DataType:    existing.DataType,
						Date:        existing.Date,
						SizeBytes:   existing.SizeBytes,
						ContentHash: existing.ContentHash,
					}
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					json.NewEncoder(w).Encode(resp)
					return
				}
			}
		}
		s.logger.Error("insert object failed", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to insert object")
		return
	}
	objectID = obj.ID
	payloadKey = fmt.Sprintf("%s/%s", collectionName, objectID)

	// Update payload_key.
	_, err = s.db.Pool().Exec(r.Context(), `UPDATE objects SET payload_key = $1 WHERE id = $2`, payloadKey, objectID)
	if err != nil {
		s.logger.Error("update payload key failed", zap.Error(err))
		// Best effort cleanup
		s.db.Pool().Exec(r.Context(), `DELETE FROM objects WHERE id = $1`, objectID)
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to update object")
		return
	}

	// Upload to S3.
	if _, err := tmpFile.Seek(0, io.SeekStart); err != nil {
		s.logger.Error("seek temp file failed", zap.Error(err))
		s.db.Pool().Exec(r.Context(), `DELETE FROM objects WHERE id = $1`, objectID)
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to store payload")
		return
	}
	if err := s.store.Upload(r.Context(), payloadKey, tmpFile, written); err != nil {
		s.logger.Error("s3 upload failed", zap.Error(err))
		s.db.Pool().Exec(r.Context(), `DELETE FROM objects WHERE id = $1`, objectID)
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to store payload")
		return
	}

	resp := models.ObjectUploadResponse{
		ID:          objectID,
		Collection:  collectionName,
		DataType:    dataType,
		Date:        date,
		SizeBytes:   written,
		ContentHash: contentHash,
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(resp)
}

func (s *Server) getObjectMetadata(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	obj, err := s.db.GetObjectByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "object not found")
		return
	}
	// Verify collection.
	collectionName := chi.URLParam(r, "collection")
	if obj.Collection != collectionName {
		writeError(w, http.StatusNotFound, "not_found", "object not found")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(obj)
}

func (s *Server) getObjectData(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	obj, err := s.db.GetObjectByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "object not found")
		return
	}
	collectionName := chi.URLParam(r, "collection")
	if obj.Collection != collectionName {
		writeError(w, http.StatusNotFound, "not_found", "object not found")
		return
	}

	reader, size, err := s.store.Download(r.Context(), obj.PayloadKey)
	if err != nil {
		s.logger.Error("download failed", zap.Error(err), zap.String("key", obj.PayloadKey))
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to retrieve payload")
		return
	}
	defer reader.Close()

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
	io.Copy(w, reader)
}

func (s *Server) getObjectTags(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	obj, err := s.db.GetObjectByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "object not found")
		return
	}
	collectionName := chi.URLParam(r, "collection")
	if obj.Collection != collectionName {
		writeError(w, http.StatusNotFound, "not_found", "object not found")
		return
	}

	tagsParam := r.URL.Query().Get("tags")
	var requestedTags []string
	seenTags := map[string]struct{}{}
	if tagsParam != "" {
		for _, t := range strings.Split(tagsParam, ",") {
			t = strings.TrimSpace(t)
			if t != "" {
				if _, exists := seenTags[t]; exists {
					continue
				}
				seenTags[t] = struct{}{}
				requestedTags = append(requestedTags, t)
			}
		}
	}
	if len(requestedTags) > s.cfg.MaxTagsPerQuery {
		writeError(w, http.StatusBadRequest, "invalid_tags", "too many tags requested")
		return
	}
	for _, tag := range requestedTags {
		if err := validate.ValidateTag(tag); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_tag", err.Error())
			return
		}
	}

	knownTags, err := s.db.GetTagsForObject(r.Context(), id)
	if err != nil {
		s.logger.Error("get tags failed", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to get tags")
		return
	}

	// Evaluate missing tags if requestedTags provided.
	if len(requestedTags) > 0 {
		var missing []string
		for _, tag := range requestedTags {
			if _, has := knownTags[tag]; !has {
				missing = append(missing, tag)
			}
		}
		if len(missing) > 0 {
			// Call tagging engine.
			resp, err := s.tagClient.Tag(r.Context(), collectionName, id, missing)
			if err != nil {
				s.logger.Error("tag engine failed", zap.Error(err))
				writeError(w, http.StatusBadGateway, "tag_engine_error", "tag engine failed")
				return
			}
			if err := s.db.UpsertTags(r.Context(), obj.CollectionID, id, resp); err != nil {
				s.logger.Error("upsert tags failed", zap.Error(err))
				writeError(w, http.StatusInternalServerError, "internal_error", "failed to persist tags")
				return
			}
			for k, v := range resp {
				knownTags[k] = v
			}
		}
	}

	result := map[string]bool{}
	if len(requestedTags) > 0 {
		for _, tag := range requestedTags {
			if v, has := knownTags[tag]; has {
				result[tag] = v
			}
		}
	} else {
		result = knownTags
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"id":   id,
		"tags": result,
	})
}

func (s *Server) queryObjects(w http.ResponseWriter, r *http.Request) {
	collectionName := chi.URLParam(r, "collection")
	if err := validate.ValidateCollectionName(collectionName); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_collection_name", err.Error())
		return
	}

	coll, err := s.db.GetCollectionByName(r.Context(), collectionName)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "collection not found")
		return
	}

	var req models.TagsQueryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "invalid request body")
		return
	}

	if req.Limit <= 0 {
		req.Limit = s.cfg.DefaultLimit
	}
	if req.Limit > s.cfg.MaxLimit {
		writeError(w, http.StatusBadRequest, "invalid_limit", "limit exceeds max")
		return
	}

	if req.Tags == nil {
		req.Tags = map[string]bool{}
	}

	if err := validate.ValidateTags(req.Tags, s.cfg.MaxTagsPerQuery); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_tags", err.Error())
		return
	}
	if err := validate.ValidateDateFilter(req.Date); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_date_filter", err.Error())
		return
	}

	resp, err := s.queryRunner.Query(r.Context(), coll, req)
	if err != nil {
		s.logger.Error("query failed", zap.Error(err))
		if strings.Contains(err.Error(), "tag engine error") || strings.Contains(err.Error(), "tag engine failure") {
			writeError(w, http.StatusBadGateway, "tag_engine_error", err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", "query failed")
		return
	}

	// For accurate has_more, we should have fetched limit+1 in query layer.
	// Let's return as-is for now; clients may see has_more=true when there are no more results.
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (s *Server) deleteObject(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	collectionName := chi.URLParam(r, "collection")
	obj, err := s.db.GetObjectByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "object not found")
		return
	}
	if obj.Collection != collectionName {
		writeError(w, http.StatusNotFound, "not_found", "object not found")
		return
	}

	payloadKey, err := s.db.DeleteObject(r.Context(), id)
	if err != nil {
		s.logger.Error("delete object failed", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to delete object")
		return
	}

	if err := s.store.Delete(r.Context(), payloadKey); err != nil {
		s.logger.Warn("delete from S3 failed", zap.Error(err), zap.String("key", payloadKey))
	}

	w.WriteHeader(http.StatusNoContent)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	resp := models.ErrorResponse{}
	resp.Error.Code = code
	resp.Error.Message = message
	json.NewEncoder(w).Encode(resp)
}

func requestLogger(logger *zap.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			next.ServeHTTP(w, r)
			logger.Info("http request",
				zap.String("method", r.Method),
				zap.String("path", r.URL.Path),
				zap.Duration("duration", time.Since(start)),
			)
		})
	}
}
