package garagetest

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
)

const (
	TestAccessKeyID = "GKTESTACCESSKEY000000000001"
	TestSecretKey   = "secret-key-value"
	TestBearerToken = "good-token"
)

type Server struct {
	HTTP *httptest.Server

	mu sync.Mutex

	ListKeysStatus       int
	GetBucketInfoStatus  int
	AllowBucketKeyStatus int
	CreateKeyStatus      int
	UpdateKeyStatus      int
	Buckets              map[string]string
	Keys                 map[string]string

	CreateKeyCalls      int
	AllowBucketKeyCalls int

	DeleteKeyCalls     atomic.Int32
	DeleteKeyFailCount atomic.Int32
	DeleteKeyFailUntil atomic.Int32
}

func New() *Server {
	s := &Server{
		ListKeysStatus: http.StatusOK,
		Buckets:        make(map[string]string),
		Keys:           make(map[string]string),
	}
	s.HTTP = httptest.NewServer(http.HandlerFunc(s.handle))
	return s
}

func (s *Server) Close() {
	s.HTTP.Close()
}

func (s *Server) URL() string {
	return s.HTTP.URL
}

func (s *Server) AddBucket(alias, id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Buckets[alias] = id
}

func (s *Server) handle(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Authorization") != "Bearer "+TestBearerToken {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	switch {
	case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/v2/ListKeys"):
		s.mu.Lock()
		status := s.ListKeysStatus
		s.mu.Unlock()
		if status != http.StatusOK {
			http.Error(w, "forbidden", status)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]struct{}{})
	case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/v2/GetBucketInfo"):
		alias := r.URL.Query().Get("globalAlias")
		s.mu.Lock()
		status := s.GetBucketInfoStatus
		id, ok := s.Buckets[alias]
		s.mu.Unlock()
		if status != 0 && status != http.StatusOK {
			http.Error(w, "bucket error", status)
			return
		}
		if !ok {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(struct {
			ID string `json:"id"`
		}{ID: id})
	case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/v2/CreateKey"):
		s.mu.Lock()
		status := s.CreateKeyStatus
		s.CreateKeyCalls++
		s.mu.Unlock()
		if status != 0 && status != http.StatusOK {
			http.Error(w, "create failed", status)
			return
		}
		s.mu.Lock()
		secret := TestSecretKey
		out := struct {
			AccessKeyID     string  `json:"accessKeyId"`
			SecretAccessKey *string `json:"secretAccessKey"`
		}{
			AccessKeyID:     TestAccessKeyID,
			SecretAccessKey: &secret,
		}
		s.Keys[out.AccessKeyID] = secret
		s.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(out)
	case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/v2/AllowBucketKey"):
		s.mu.Lock()
		status := s.AllowBucketKeyStatus
		s.AllowBucketKeyCalls++
		s.mu.Unlock()
		if status != 0 && status != http.StatusOK {
			http.Error(w, "allow failed", status)
			return
		}
		w.WriteHeader(http.StatusOK)
	case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/v2/DeleteKey"):
		attempt := s.DeleteKeyFailCount.Add(1)
		s.DeleteKeyCalls.Add(1)
		if attempt <= s.DeleteKeyFailUntil.Load() {
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}
		id := r.URL.Query().Get("id")
		s.mu.Lock()
		delete(s.Keys, id)
		s.mu.Unlock()
		w.WriteHeader(http.StatusOK)
	case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/v2/UpdateKey"):
		s.mu.Lock()
		status := s.UpdateKeyStatus
		s.mu.Unlock()
		if status != 0 && status != http.StatusOK {
			http.Error(w, "update failed", status)
			return
		}
		w.WriteHeader(http.StatusOK)
	default:
		http.NotFound(w, r)
	}
}
