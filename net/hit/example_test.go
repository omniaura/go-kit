package hit_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/omniaura/go-kit/net/hit"
)

func ExampleRequest_Cache() {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(testItem{ID: 7, Name: "cached"})
	}))
	defer server.Close()

	hot, _ := hit.NewMapCache(hit.WithTTL(time.Minute))
	warm, _ := hit.NewMapCache(hit.WithTTL(5 * time.Minute))
	cache := hit.NewLayeredCache(hot, warm)

	var out testItem
	_ = hit.GET[testItem](server.URL).
		Cache(cache).
		Key(hit.Field("provider", "example")).
		Do(context.Background(), &out)

	fmt.Println(out.Name)
	// Output: cached
}

func ExampleRequest_Rate() {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(testItem{ID: 1, Name: "limited"})
	}))
	defer server.Close()

	limiter, _ := hit.RPS(10)

	var out testItem
	_ = hit.GET[testItem](server.URL).
		Rate(limiter).
		Do(context.Background(), &out)

	fmt.Println(out.Name)
	// Output: limited
}
