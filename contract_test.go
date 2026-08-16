// Fails when the SDK stops covering every route in the OpenAPI spec.
//
// Docs list inboxes at /mailboxes and every other operation at /mailbox/{id}.
// The client still calls the /mailboxes/{id} alias; those requests are mapped
// onto the documented singular templates.
//
// The spec lives in this module (spec/openapi.json). When Docs/ is checked
// out next to Sdk/ in the TropMail workspace, the test also asserts they match.
package tropmail

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"
)

const (
	specRelPath    = "spec/openapi.json"
	docSpecRelPath = "../../Docs/Doc/openapi.json"
)

var httpMethods = map[string]bool{
	"get": true, "post": true, "put": true, "patch": true, "delete": true,
}

// pathTemplates maps a concrete request path back to its OpenAPI template.
var pathTemplates = []struct {
	pattern  *regexp.Regexp
	template string
}{
	{regexp.MustCompile(`^/mailbox/[^/]+/emails/[^/]+/(text|html|markdown)$`), "/mailbox/{id}/emails/{emailId}/{view}"},
	{regexp.MustCompile(`^/mailbox/[^/]+/emails/search$`), "/mailbox/{id}/emails/search"},
	{regexp.MustCompile(`^/mailbox/[^/]+/emails/[^/]+/scan-attachments$`), "/mailbox/{id}/emails/{emailId}/scan-attachments"},
	{regexp.MustCompile(`^/mailbox/[^/]+/emails/[^/]+/download-attachments$`), "/mailbox/{id}/emails/{emailId}/download-attachments"},
	{regexp.MustCompile(`^/mailbox/[^/]+/attachments/[^/]+/scan$`), "/mailbox/{id}/attachments/{attId}/scan"},
	{regexp.MustCompile(`^/mailbox/[^/]+/attachments/[^/]+/download$`), "/mailbox/{id}/attachments/{attId}/download"},
	{regexp.MustCompile(`^/mailbox/[^/]+/attachments/[^/]+$`), "/mailbox/{id}/attachments/{attId}"},
	{regexp.MustCompile(`^/mailbox/[^/]+/emails/[^/]+$`), "/mailbox/{id}/emails/{emailId}"},
	{regexp.MustCompile(`^/mailbox/[^/]+/emails$`), "/mailbox/{id}/emails"},
	{regexp.MustCompile(`^/mailbox/[^/]+$`), "/mailbox/{id}"},
}

func templatize(path string) string {
	path = strings.TrimPrefix(path, "/api/v1")
	if strings.HasPrefix(path, "/mailboxes/") {
		path = "/mailbox/" + strings.TrimPrefix(path, "/mailboxes/")
	}
	for _, rule := range pathTemplates {
		if rule.pattern.MatchString(path) {
			return rule.template
		}
	}
	return path
}

func loadSpec(t *testing.T, path string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var spec map[string]any
	if err := json.Unmarshal(raw, &spec); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	return spec
}

func specOperations(t *testing.T) map[string]bool {
	t.Helper()
	spec := loadSpec(t, specRelPath)
	paths, ok := spec["paths"].(map[string]any)
	if !ok {
		t.Fatal("spec has no paths object")
	}

	operations := map[string]bool{}
	for path, methods := range paths {
		entries, ok := methods.(map[string]any)
		if !ok {
			continue
		}
		for method := range entries {
			if httpMethods[strings.ToLower(method)] {
				operations[strings.ToUpper(method)+" "+path] = true
			}
		}
	}
	return operations
}

// anyPayload is a superset of every response shape, so one body satisfies any call.
var anyPayload = map[string]any{
	"id":            "11111111-1111-1111-1111-111111111111",
	"email":         "user@tropmail.com",
	"timestamp":     "2026-01-01T00:00:00Z",
	"from":          map[string]any{"name": "Sender", "address": "sender@example.com"},
	"attachment_id": "22222222-2222-2222-2222-222222222222",
	"mailboxes":     []any{},
}

func exerciseEveryMethod(t *testing.T) map[string]bool {
	t.Helper()
	seen := map[string]bool{}

	client, _ := testServer(t, func(w http.ResponseWriter, r *http.Request) {
		seen[r.Method+" "+templatize(r.URL.Path)] = true
		if strings.HasSuffix(r.URL.Path, "/download") {
			w.Header().Set("Content-Type", "application/pdf")
			w.Header().Set("Content-Disposition", `attachment; filename="invoice.pdf"`)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("%PDF-1.4 mock"))
			return
		}
		if strings.HasSuffix(r.URL.Path, "-attachments") {
			writeEnvelope(t, w, http.StatusOK, []any{})
			return
		}
		writeEnvelope(t, w, http.StatusOK, anyPayload)
	})

	ctx := context.Background()
	mustSucceed := func(label string, err error) {
		if err != nil {
			t.Fatalf("%s: %v", label, err)
		}
	}

	_, err := client.Health(ctx)
	mustSucceed("Health", err)
	_, err = client.Mailboxes.List(ctx)
	mustSucceed("Mailboxes.List", err)
	_, err = client.Mailboxes.Get(ctx, testMailboxID)
	mustSucceed("Mailboxes.Get", err)
	_, err = client.Emails.List(ctx, ListOptions{MailboxID: testMailboxID})
	mustSucceed("Emails.List", err)
	_, err = client.Emails.Search(ctx, "q", ListOptions{MailboxID: testMailboxID})
	mustSucceed("Emails.Search", err)
	_, err = client.Emails.Get(ctx, testMailboxID, "id", GetOptions{})
	mustSucceed("Emails.Get", err)
	_, err = client.Emails.Get(ctx, testMailboxID, "id", GetOptions{View: ViewMarkdown})
	mustSucceed("Emails.Get markdown", err)
	_, err = client.Emails.SetState(ctx, testMailboxID, "id", StateOpen)
	mustSucceed("Emails.SetState", err)
	_, err = client.Emails.ScanAttachments(ctx, testMailboxID, "id")
	mustSucceed("Emails.ScanAttachments", err)
	_, err = client.Emails.DownloadAttachments(ctx, testMailboxID, "id")
	mustSucceed("Emails.DownloadAttachments", err)
	_, err = client.Attachments.Get(ctx, testMailboxID, "aid")
	mustSucceed("Attachments.Get", err)
	_, err = client.Attachments.Scan(ctx, testMailboxID, "aid")
	mustSucceed("Attachments.Scan", err)
	body, err := client.Attachments.Open(ctx, testMailboxID, "aid")
	mustSucceed("Attachments.Open", err)
	_ = body.Close()

	return seen
}

func TestContractSpecExists(t *testing.T) {
	if _, err := os.Stat(specRelPath); err != nil {
		t.Fatalf("missing spec: %v", err)
	}
}

func TestContractSpecMatchesDocSite(t *testing.T) {
	if _, err := os.Stat(filepath.Clean(docSpecRelPath)); err != nil {
		t.Skip("Docs is not checked out next to the SDK")
	}
	if !reflect.DeepEqual(loadSpec(t, docSpecRelPath), loadSpec(t, specRelPath)) {
		t.Error("spec/openapi.json drifted from Docs/Doc/openapi.json — run 'make sync-spec'")
	}
}

func TestContractEveryRouteIsCovered(t *testing.T) {
	covered := exerciseEveryMethod(t)

	var missing []string
	for operation := range specOperations(t) {
		if !covered[operation] {
			missing = append(missing, operation)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Errorf("SDK does not cover: %v", missing)
	}
}

func TestContractNoUndocumentedRoutes(t *testing.T) {
	operations := specOperations(t)

	var extra []string
	for operation := range exerciseEveryMethod(t) {
		if !operations[operation] {
			extra = append(extra, operation)
		}
	}
	sort.Strings(extra)
	if len(extra) > 0 {
		t.Errorf("SDK calls undocumented routes: %v", extra)
	}
}
