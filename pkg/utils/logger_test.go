package utils

// TODO: Implement remote logging tests

// import (
// 	"encoding/json"
// 	"io"
// 	"net/http"
// 	"net/http/httptest"
// 	"sync/atomic"
// 	"testing"
// 	"time"
// )

// func TestLoggerRemoteShipping_Generic(t *testing.T) {
// 	var gotCount int32
// 	var lastBody []map[string]interface{}
// 	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
// 		atomic.AddInt32(&gotCount, 1)
// 		defer r.Body.Close()
// 		b, _ := io.ReadAll(r.Body)
// 		_ = json.Unmarshal(b, &lastBody)
// 		w.WriteHeader(200)
// 	}))
// 	defer srv.Close()

// 	l := NewLogger(false, false)
// 	// l.EnableRemoteShipping(srv.URL, map[string]string{"X-Test": "1"}, "generic")

// 	l.Info("hello")
// 	l.Error("world")

// 	time.Sleep(2500 * time.Millisecond) // allow shipper batch to flush
// 	if atomic.LoadInt32(&gotCount) == 0 {
// 		t.Fatalf("expected at least one POST")
// 	}
// 	if len(lastBody) == 0 {
// 		t.Fatalf("expected array payload")
// 	}
// }
