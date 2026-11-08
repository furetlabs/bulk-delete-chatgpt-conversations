package chatgpt

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/cdproto/network"
)

func TestReadCookieValueSupportsChunkedSessionCookies(t *testing.T) {
	token := "part-0-part-1-part-2"

	got := readCookieValue([]*network.Cookie{
		{Name: sessionCookie + ".1", Value: "part-1-"},
		{Name: csrfCookie, Value: "csrf-token"},
		{Name: sessionCookie + ".0", Value: "part-0-"},
		{Name: sessionCookie + ".2", Value: "part-2"},
	}, sessionCookie)

	if got != token {
		t.Fatalf("readCookieValue(chunked) = %q, want %q", got, token)
	}
}

func TestReadCookieValueReturnsEmptyWhenChunkSequenceHasGap(t *testing.T) {
	got := readCookieValue([]*network.Cookie{
		{Name: sessionCookie + ".0", Value: "part-0-"},
		{Name: sessionCookie + ".2", Value: "part-2"},
	}, sessionCookie)

	if got != "" {
		t.Fatalf("readCookieValue(missing chunk) = %q, want empty string", got)
	}
}

func TestSplitCookieChunksRoundTripsLargeSessionToken(t *testing.T) {
	token := strings.Repeat("a", cookieChunkLen*2+25)

	cookies := splitCookieChunks(sessionCookie, token)
	if len(cookies) != 3 {
		t.Fatalf("splitCookieChunks() count = %d, want 3", len(cookies))
	}
	if cookies[0].Name != sessionCookie+".0" {
		t.Fatalf("first chunk name = %q, want %q", cookies[0].Name, sessionCookie+".0")
	}
	if cookies[1].Name != sessionCookie+".1" {
		t.Fatalf("second chunk name = %q, want %q", cookies[1].Name, sessionCookie+".1")
	}
	if cookies[2].Name != sessionCookie+".2" {
		t.Fatalf("third chunk name = %q, want %q", cookies[2].Name, sessionCookie+".2")
	}

	var browserCookies []*network.Cookie
	for _, cookie := range cookies {
		browserCookies = append(browserCookies, &network.Cookie{
			Name:  cookie.Name,
			Value: cookie.Value,
		})
	}

	if got := readCookieValue(browserCookies, sessionCookie); got != token {
		t.Fatalf("readCookieValue(round trip) = %d bytes, want %d bytes", len(got), len(token))
	}
}

func TestInitializeHTTPClientSeedsChunkedSessionCookies(t *testing.T) {
	token := strings.Repeat("b", cookieChunkLen+10)

	client, err := New(Config{SessionToken: token})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if err := client.initializeHTTPClient(); err != nil {
		t.Fatalf("initializeHTTPClient() error = %v", err)
	}

	req, err := http.NewRequest(http.MethodGet, chatGPTBaseURL, nil)
	if err != nil {
		t.Fatalf("http.NewRequest() error = %v", err)
	}

	cookies := client.httpClient.Jar.Cookies(req.URL)
	if len(cookies) != 2 {
		t.Fatalf("jar cookie count = %d, want 2", len(cookies))
	}
	if cookies[0].Name != sessionCookie+".0" {
		t.Fatalf("first jar cookie name = %q, want %q", cookies[0].Name, sessionCookie+".0")
	}
	if cookies[1].Name != sessionCookie+".1" {
		t.Fatalf("second jar cookie name = %q, want %q", cookies[1].Name, sessionCookie+".1")
	}
}

func TestBrowserCommandContextUsesBrowserContextAndPreservesDeadline(t *testing.T) {
	type ctxKey string

	sourceCtx, sourceCancel := context.WithTimeout(context.WithValue(context.Background(), ctxKey("source"), "source"), 5*time.Minute)
	defer sourceCancel()

	browserCtx := context.WithValue(context.Background(), ctxKey("browser"), "browser")
	client := &Client{browserCtx: browserCtx}

	gotCtx, cancel := client.browserCommandContext(sourceCtx)
	defer cancel()

	if got := gotCtx.Value(ctxKey("browser")); got != "browser" {
		t.Fatalf("browserCommandContext() browser value = %v, want browser context value", got)
	}
	if got := gotCtx.Value(ctxKey("source")); got != nil {
		t.Fatalf("browserCommandContext() source value = %v, want nil because browser context should be used", got)
	}

	gotDeadline, ok := gotCtx.Deadline()
	if !ok {
		t.Fatalf("browserCommandContext() deadline missing")
	}
	wantDeadline, _ := sourceCtx.Deadline()
	if !gotDeadline.Equal(wantDeadline) {
		t.Fatalf("browserCommandContext() deadline = %v, want %v", gotDeadline, wantDeadline)
	}
}

func TestCachedConversationsReturnsStoredVisibleItems(t *testing.T) {
	cachePath := filepath.Join(t.TempDir(), "conversations.json")
	t.Setenv(conversationCacheEnvVar, cachePath)

	_, err := SaveConversationCache(ConversationCache{
		Conversations: []Conversation{
			{ID: "visible", Title: "Visible"},
			{ID: "hidden", Title: "Hidden"},
		},
		Hidden: map[string]ConversationCacheEntry{
			"hidden": {
				Visibility: ConversationVisibilityDeleted,
				UpdatedAt:  time.Date(2026, time.March, 25, 9, 0, 0, 0, time.UTC),
			},
		},
	})
	if err != nil {
		t.Fatalf("SaveConversationCache() error = %v", err)
	}

	client := &Client{}
	got, err := client.CachedConversations()
	if err != nil {
		t.Fatalf("CachedConversations() error = %v", err)
	}

	want := []Conversation{{ID: "visible", Title: "Visible"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("CachedConversations() = %#v, want %#v", got, want)
	}
}

func TestListConversationsRefreshesCacheAndFiltersHiddenItems(t *testing.T) {
	cachePath := filepath.Join(t.TempDir(), "conversations.json")
	t.Setenv(conversationCacheEnvVar, cachePath)

	_, err := SaveConversationCache(ConversationCache{
		Conversations: []Conversation{{ID: "cached-only", Title: "Cached only"}},
		Hidden: map[string]ConversationCacheEntry{
			"33333333-3333-4333-8333-333333333333": {
				Visibility: ConversationVisibilityArchived,
				UpdatedAt:  time.Date(2026, time.March, 25, 9, 30, 0, 0, time.UTC),
			},
		},
	})
	if err != nil {
		t.Fatalf("SaveConversationCache() error = %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/backend-api/conversations" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}

		offset := r.URL.Query().Get("offset")
		total := 3
		payload := conversationList{Total: &total}
		switch offset {
		case "0":
			payload.Items = []Conversation{
				{ID: "11111111-1111-4111-8111-111111111111", Title: "Visible 1"},
				{ID: "33333333-3333-4333-8333-333333333333", Title: "Archived locally"},
			}
		case "2":
			payload.Items = []Conversation{
				{ID: "22222222-2222-4222-8222-222222222222", Title: "Visible 2"},
			}
		default:
			payload.Items = []Conversation{}
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(payload); err != nil {
			t.Fatalf("json.NewEncoder().Encode() error = %v", err)
		}
	}))
	defer server.Close()

	client := newBackendTestClient(t, server)

	got, err := client.ListConversations(context.Background(), 2)
	if err != nil {
		t.Fatalf("ListConversations() error = %v", err)
	}

	want := []Conversation{
		{ID: "11111111-1111-4111-8111-111111111111", Title: "Visible 1"},
		{ID: "22222222-2222-4222-8222-222222222222", Title: "Visible 2"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ListConversations() = %#v, want %#v", got, want)
	}

	cache, _, err := LoadConversationCache()
	if err != nil {
		t.Fatalf("LoadConversationCache() error = %v", err)
	}
	if !reflect.DeepEqual(cache.Conversations, want) {
		t.Fatalf("cached conversations = %#v, want %#v", cache.Conversations, want)
	}
	if _, ok := cache.Hidden["33333333-3333-4333-8333-333333333333"]; !ok {
		t.Fatal("hidden archived conversation was not preserved after refresh")
	}
}

func TestConversationMutationsUpdateLocalCacheVisibility(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		id         string
		visibility ConversationVisibility
		call       func(context.Context, *Client, string) error
	}{
		{
			name:       "archive",
			method:     http.MethodPatch,
			id:         "22222222-2222-4222-8222-222222222222",
			visibility: ConversationVisibilityArchived,
			call: func(ctx context.Context, client *Client, id string) error {
				return client.ArchiveConversation(ctx, id)
			},
		},
		{
			name:       "delete",
			method:     http.MethodPatch,
			id:         "11111111-1111-4111-8111-111111111111",
			visibility: ConversationVisibilityDeleted,
			call: func(ctx context.Context, client *Client, id string) error {
				return client.DeleteConversation(ctx, id)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("CHATGPT_BULK_PROTECTION_FILE", filepath.Join(t.TempDir(), "protected.json"))
			if err := InitializeProtections(); err != nil {
				t.Fatal(err)
			}
			cachePath := filepath.Join(t.TempDir(), "conversations.json")
			t.Setenv(conversationCacheEnvVar, cachePath)

			_, err := SaveConversationCache(ConversationCache{
				Conversations: []Conversation{
					{ID: tc.id, Title: "Mutated"},
					{ID: "keep-me", Title: "Keep me"},
				},
			})
			if err != nil {
				t.Fatalf("SaveConversationCache() error = %v", err)
			}

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != tc.method {
					t.Fatalf("request method = %s, want %s", r.Method, tc.method)
				}
				if r.URL.Path != "/backend-api/conversation/"+tc.id {
					t.Fatalf("request path = %s, want /backend-api/conversation/%s", r.URL.Path, tc.id)
				}
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{}`))
			}))
			defer server.Close()

			client := newBackendTestClient(t, server)
			if err := tc.call(context.Background(), client, tc.id); err != nil {
				t.Fatalf("mutation error = %v", err)
			}

			cache, _, err := LoadConversationCache()
			if err != nil {
				t.Fatalf("LoadConversationCache() error = %v", err)
			}
			if len(cache.Conversations) != 1 || cache.Conversations[0].ID != "keep-me" {
				t.Fatalf("cached conversations after mutation = %#v, want only keep-me", cache.Conversations)
			}
			entry, ok := cache.Hidden[tc.id]
			if !ok {
				t.Fatalf("hidden entry missing for %s", tc.id)
			}
			if entry.Visibility != tc.visibility {
				t.Fatalf("hidden visibility = %q, want %q", entry.Visibility, tc.visibility)
			}
		})
	}
}

func newBackendTestClient(t *testing.T, server *httptest.Server) *Client {
	t.Helper()

	serverURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("url.Parse(%q) error = %v", server.URL, err)
	}

	httpClient := server.Client()
	httpClient.Transport = rewriteRequestTransport{
		baseURL: serverURL,
		next:    http.DefaultTransport,
	}

	return &Client{
		httpClient:        httpClient,
		allowMutations:    true,
		inventoryComplete: true,
		inventoryIDs:      map[string]struct{}{"11111111-1111-4111-8111-111111111111": {}, "22222222-2222-4222-8222-222222222222": {}},
		accessToken:       "test-access-token",
	}
}

type rewriteRequestTransport struct {
	baseURL *url.URL
	next    http.RoundTripper
}

func (t rewriteRequestTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	clone.URL.Scheme = t.baseURL.Scheme
	clone.URL.Host = t.baseURL.Host
	clone.Host = t.baseURL.Host
	return t.next.RoundTrip(clone)
}

func TestBackendErrorsAndLogsDoNotExposeResponseSecrets(t *testing.T) {
	secret := "synthetic-secret-value"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, secret, http.StatusInternalServerError) }))
	defer server.Close()
	client := newBackendTestClient(t, server)
	client.debug = true
	_, err := client.doBackendRequest(context.Background(), http.MethodGet, backendAPIURL+"/conversations", nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), secret) || strings.Contains(strings.Join(client.Logs(), " "), secret) {
		t.Fatal("secret leaked")
	}
}

func TestListRejectsMissingItems(t *testing.T) {
	t.Setenv(conversationCacheEnvVar, filepath.Join(t.TempDir(), "cache.json"))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()
	client := newBackendTestClient(t, server)
	if _, err := client.ListConversations(context.Background(), 2); err == nil {
		t.Fatal("accepted missing items")
	}
}

func TestListRejectsIncompleteOrUnstablePagination(t *testing.T) {
	tests := []struct {
		name  string
		pages map[string]string
	}{
		{"missing_total", map[string]string{"0": `{"items":[]}`}},
		{"truncated", map[string]string{"0": `{"items":[{"id":"11111111-1111-4111-8111-111111111111"}],"total":2}`, "1": `{"items":[],"total":2}`}},
		{"duplicate", map[string]string{"0": `{"items":[{"id":"11111111-1111-4111-8111-111111111111"}],"total":2}`, "1": `{"items":[{"id":"11111111-1111-4111-8111-111111111111"}],"total":2}`}},
		{"changed_total", map[string]string{"0": `{"items":[{"id":"11111111-1111-4111-8111-111111111111"}],"total":2}`, "1": `{"items":[{"id":"22222222-2222-4222-8222-222222222222"}],"total":3}`}},
		{"missing_id", map[string]string{"0": `{"items":[{}],"total":1}`}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(conversationCacheEnvVar, filepath.Join(t.TempDir(), "cache.json"))
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if page, ok := tc.pages[r.URL.Query().Get("offset")]; ok {
					_, _ = w.Write([]byte(page))
				} else {
					_, _ = w.Write([]byte(`{"items":[],"total":2}`))
				}
			}))
			defer server.Close()
			c := newBackendTestClient(t, server)
			if _, err := c.ListConversations(context.Background(), 1); err == nil {
				t.Fatal("expected incomplete inventory error")
			}
		})
	}
}

func TestFailedRefreshInvalidatesInventoryForDeletion(t *testing.T) {
	t.Setenv(conversationCacheEnvVar, filepath.Join(t.TempDir(), "cache.json"))
	t.Setenv("CHATGPT_BULK_PROTECTION_FILE", filepath.Join(t.TempDir(), "protected.json"))
	if err := InitializeProtections(); err != nil {
		t.Fatal(err)
	}
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	c := newBackendTestClient(t, server)
	if _, err := c.ListConversations(context.Background(), 100); err == nil {
		t.Fatal("refresh should fail")
	}
	before := requests
	if err := c.DeleteConversation(context.Background(), "11111111-1111-4111-8111-111111111111"); err == nil || !strings.Contains(err.Error(), "incomplete") {
		t.Fatalf("expected incomplete inventory: %v", err)
	}
	if requests != before {
		t.Fatal("delete reached server after failed inventory")
	}
}

func TestDeleteRejectsTargetOutsideInventory(t *testing.T) {
	t.Setenv("CHATGPT_BULK_PROTECTION_FILE", filepath.Join(t.TempDir(), "protected.json"))
	if err := InitializeProtections(); err != nil {
		t.Fatal(err)
	}
	c := &Client{allowMutations: true, inventoryComplete: true}
	err := c.DeleteConversation(context.Background(), "11111111-1111-4111-8111-111111111111")
	if err == nil || !strings.Contains(err.Error(), "not in inventory") {
		t.Fatalf("unknown target accepted: %v", err)
	}
}

func TestBrowserCommandContextPropagatesCancellation(t *testing.T) {
	source, cancel := context.WithCancel(context.Background())
	c := &Client{browserCtx: context.Background()}
	got, stop := c.browserCommandContext(source)
	defer stop()
	cancel()
	select {
	case <-got.Done():
	case <-time.After(time.Second):
		t.Fatal("browser context ignored cancellation")
	}
}

func TestBackendRejectsOversizedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(bytes.Repeat([]byte("x"), 9<<20)) }))
	defer server.Close()
	c := newBackendTestClient(t, server)
	_, err := c.doBackendRequest(context.Background(), http.MethodGet, backendAPIURL+"/conversations", nil)
	if err == nil || !strings.Contains(err.Error(), "too large") {
		t.Fatalf("expected bounded response error: %v", err)
	}
}

func TestListRejectsUnknownIdentifierFormat(t *testing.T) {
	t.Setenv(conversationCacheEnvVar, filepath.Join(t.TempDir(), "cache.json"))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"items":[{"id":"unexpected"}],"total":1}`))
	}))
	defer server.Close()
	c := newBackendTestClient(t, server)
	if _, err := c.ListConversations(context.Background(), 1); err == nil {
		t.Fatal("accepted unidentifiable item")
	}
}

func TestProtectionChangeWaitsForInFlightMutation(t *testing.T) {
	t.Setenv(conversationCacheEnvVar, filepath.Join(t.TempDir(), "cache.json"))
	t.Setenv("CHATGPT_BULK_PROTECTION_FILE", filepath.Join(t.TempDir(), "protected.json"))
	if err := InitializeProtections(); err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { close(started); <-release; w.WriteHeader(http.StatusOK) }))
	defer server.Close()
	c := newBackendTestClient(t, server)
	id := "11111111-1111-4111-8111-111111111111"
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	deleted := make(chan error, 1)
	go func() { deleted <- c.DeleteConversation(ctx, id) }()
	select {
	case <-started:
	case <-ctx.Done():
		close(release)
		t.Fatal("request did not start")
	}
	protected := make(chan error, 1)
	go func() { protected <- SetProtected(id, true) }()
	select {
	case err := <-protected:
		close(release)
		t.Fatalf("protection changed while request was in flight: %v", err)
	case <-time.After(80 * time.Millisecond):
	}
	close(release)
	if err := <-deleted; err != nil {
		t.Fatal(err)
	}
	if err := <-protected; err != nil {
		t.Fatal(err)
	}
	p, err := IsProtected(id)
	if err != nil || !p {
		t.Fatal("protection was not saved")
	}
}

func TestBackendDoesNotFollowRedirects(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		http.Redirect(w, r, "http://example.invalid/secret", http.StatusFound)
	}))
	defer server.Close()
	c, err := New(Config{SessionToken: "fake", AccessToken: "fake"})
	if err != nil {
		t.Fatal(err)
	}
	if err = c.initializeHTTPClient(); err != nil {
		t.Fatal(err)
	}
	serverURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	c.httpClient.Transport = rewriteRequestTransport{baseURL: serverURL, next: http.DefaultTransport}
	_, err = c.doBackendRequest(context.Background(), http.MethodGet, backendAPIURL+"/conversations", nil)
	if err == nil || requests != 1 {
		t.Fatalf("redirect was followed: count=%d error=%v", requests, err)
	}
}
