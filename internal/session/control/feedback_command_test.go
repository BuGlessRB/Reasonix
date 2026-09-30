package control

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"reasonix/internal/base/i18n"
	"reasonix/internal/base/testenv"
	"reasonix/internal/contract/event"
	"reasonix/internal/platform/feedback"
)

type feedbackRig struct {
	c       *Controller
	mu      sync.Mutex
	texts   []string
	posts   int
	status  int
	mine    string
	respond string
	onPost  func(body string)
}

func newFeedbackRig(t *testing.T, surface feedback.Surface, withService bool) *feedbackRig {
	t.Helper()
	r := &feedbackRig{respond: `{"receipt":"FB-7K3M-9QX2","status":"received","installToken":"t","createdAt":"2026-09-30T08:00:00Z"}`}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		r.mu.Lock()
		defer r.mu.Unlock()
		if req.Method == http.MethodPost {
			r.posts++
			if r.onPost != nil {
				raw, _ := io.ReadAll(req.Body)
				r.onPost(string(raw))
			}
		}
		if r.status != 0 {
			w.WriteHeader(r.status)
		}
		if req.Method == http.MethodGet {
			_, _ = io.WriteString(w, r.mine)
			return
		}
		_, _ = io.WriteString(w, r.respond)
	}))
	t.Cleanup(srv.Close)
	opts := Options{Feedback: FeedbackOptions{Surface: surface}, Sink: event.FuncSink(func(e event.Event) {
		if e.Kind == event.Notice {
			r.mu.Lock()
			r.texts = append(r.texts, e.Text)
			r.mu.Unlock()
		}
	})}
	if withService {
		svc, err := feedback.New(feedback.Config{Home: testenv.TempDir(t), Base: srv.URL, HTTP: srv.Client(), Backoff: []time.Duration{}})
		if err != nil {
			t.Fatal(err)
		}
		opts.Feedback.Service = svc
	}
	r.c = New(opts)
	return r
}

func (r *feedbackRig) last(t *testing.T, contains string) string {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		r.mu.Lock()
		for _, s := range r.texts {
			if strings.Contains(s, contains) {
				r.mu.Unlock()
				return s
			}
		}
		r.mu.Unlock()
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("no notice containing %q; got %q", contains, r.texts)
	return ""
}

func (r *feedbackRig) postCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.posts
}

func TestFeedbackCommandNeedsANicknameThenAnExplicitYes(t *testing.T) {
	r := newFeedbackRig(t, feedback.SurfaceTUI, true)
	r.c.Submit("/feedback bug the sidebar forgets me")
	r.last(t, "set a nickname first")
	r.c.Submit("/feedback name kim")
	r.last(t, "nickname set to kim")
	r.c.Submit("/feedback bug the sidebar forgets me")
	if got := r.last(t, "PUBLIC GitHub issue"); !strings.Contains(got, "kim") || !strings.Contains(got, "--yes") {
		t.Fatalf("notice = %q", got)
	}
	if r.postCount() != 0 {
		t.Fatal("something was sent without --yes")
	}
	r.c.Submit("/feedback bug --yes the sidebar forgets me")
	r.last(t, "Receipt FB-7K3M-9QX2")
	if r.postCount() != 1 {
		t.Fatalf("posts = %d", r.postCount())
	}
}

func TestFeedbackCommandListShowsStatusAndOfflineHonesty(t *testing.T) {
	r := newFeedbackRig(t, feedback.SurfaceTUI, true)
	r.mine = `{"items":[{"receipt":"FB-7K3M-9QX2","category":"bug","titleSnippet":"Sidebar","status":"fixed","issueNumber":11350,"resolvedVersion":"v2.25.0","createdAt":"2026-09-30T08:00:00Z","updatedAt":"2026-10-02T08:00:00Z"},{"receipt":"FB-2H8P-4WD7","category":"idea","titleSnippet":"Export","status":"duplicate","duplicateOf":11302,"createdAt":"2026-09-29T08:00:00Z","updatedAt":"2026-09-29T08:00:00Z"}]}`
	r.c.Submit("/feedback list")
	r.last(t, "no feedback sent")
	r.c.Submit("/feedback name kim")
	r.c.Submit("/feedback bug --yes x")
	r.last(t, "Receipt")
	r.c.Submit("/feedback list")
	got := r.last(t, "fixed in v2.25.0")
	if !strings.Contains(got, "#11350") || !strings.Contains(got, "duplicate of #11302") {
		t.Fatalf("list = %q", got)
	}
}

func TestFeedbackCommandRefusalsAreSaidFromTheirIdentity(t *testing.T) {
	r := newFeedbackRig(t, feedback.SurfaceTUI, true)
	r.c.Submit("/feedback name kim")
	r.status = http.StatusTooManyRequests
	r.respond = `{"error":{"code":"feedback.rate_limited"}}`
	r.c.Submit("/feedback bug --yes x")
	r.last(t, "too many submissions")
	r.c.Submit("/feedback rant --yes x")
	r.last(t, "category is not acceptable (bad_value)")
}

func TestFeedbackCommandIsUnavailableWithoutASurfaceOrService(t *testing.T) {
	for name, r := range map[string]*feedbackRig{
		"no surface": newFeedbackRig(t, "", true),
		"no service": newFeedbackRig(t, feedback.SurfaceTUI, false),
	} {
		r.c.Submit("/feedback bug --yes x")
		r.last(t, "not available")
		if r.postCount() != 0 {
			t.Errorf("%s: sent anyway", name)
		}
	}
}

func TestFeedbackIsCompletedAsACommand(t *testing.T) {
	items, _ := SlashArgItems("/feedback ", ArgData{})
	labels := []string{}
	for _, it := range items {
		labels = append(labels, it.Label)
	}
	if strings.Join(labels, ",") != "bug,idea,question,other,list,name" {
		t.Fatalf("labels = %v", labels)
	}
	found := false
	for _, it := range SubmitSlashCommands(i18n.M) {
		found = found || it.Label == "/feedback"
	}
	if !found {
		t.Fatal("/feedback is missing from the built-in catalogue")
	}
}

func TestFeedbackYesOnlyCountsRightAfterTheCategory(t *testing.T) {
	r := newFeedbackRig(t, feedback.SurfaceTUI, true)
	r.c.Submit("/feedback name kim")
	r.last(t, "nickname set")
	r.c.Submit("/feedback bug the flag --yes appears inside my report")
	r.last(t, "Nothing was sent")
	r.c.Submit("/feedback bug please --yes")
	time.Sleep(100 * time.Millisecond)
	if r.postCount() != 0 {
		t.Fatal("a report that mentions --yes was sent without a preview")
	}
}

func TestFeedbackKeepsTheLinesOfAMultiLineReport(t *testing.T) {
	r := newFeedbackRig(t, feedback.SurfaceTUI, true)
	r.c.Submit("/feedback name kim")
	r.last(t, "nickname set")
	var got string
	r.onPost = func(body string) { got = body }
	r.c.Submit("/feedback bug --yes line one\n  indented line two\n\nline four")
	r.last(t, "Receipt")
	if !strings.Contains(got, `line one\n  indented line two\n\nline four`) {
		t.Fatalf("body on the wire = %s", got)
	}
}
