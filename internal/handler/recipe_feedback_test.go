package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/AntonKilk/cooking-helper/internal/domain"
	"github.com/AntonKilk/cooking-helper/internal/repository"
	"github.com/AntonKilk/cooking-helper/internal/service"
)

// stubFeedbackSetter is an in-memory feedbackSetter for the handler tests. It
// applies the absolute state to the stored recipe (so idempotency is
// observable) and returns the updated recipe. A nil entry / missing id returns
// repository.ErrNotFound; err forces a hard failure.
type stubFeedbackSetter struct {
	byID map[string]*domain.Recipe
	err  error
}

func (s stubFeedbackSetter) SetFeedback(_ context.Context, id string, fb domain.Feedback) (*domain.Recipe, error) {
	if s.err != nil {
		return nil, s.err
	}
	rec, ok := s.byID[id]
	if !ok {
		return nil, repository.ErrNotFound
	}
	if !fb.Liked && !fb.Disliked && !fb.CookAgain {
		rec.Feedback = nil
	} else {
		rec.Feedback = &fb
	}
	return rec, nil
}

// Compile-time guards: the real service and the stub satisfy the narrow
// interface the handler depends on.
var (
	_ feedbackSetter = (*service.RecipeService)(nil)
	_ feedbackSetter = stubFeedbackSetter{}
)

func newFeedbackHandler(t *testing.T, setter feedbackSetter) *recipeHandlers {
	t.Helper()
	rd := &renderer{tmpl: testTemplates(t), bundle: testBundle(t)}
	return &recipeHandlers{rd: rd, recipes: stubRecipeReader{byID: testRecipes()}, feedback: setter}
}

func postFeedback(id, body string) (*httptest.ResponseRecorder, *http.Request) {
	req := httptest.NewRequest(http.MethodPost, "/recipe/"+id+"/feedback", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetPathValue("id", id)
	return httptest.NewRecorder(), req
}

func TestFeedbackSetsAndRendersActive(t *testing.T) {
	rec := &domain.Recipe{ID: "r1", Title: "Soup"}
	h := newFeedbackHandler(t, stubFeedbackSetter{byID: map[string]*domain.Recipe{"r1": rec}})

	w, req := postFeedback("r1", "liked=true")
	h.Feedback(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if rec.Feedback == nil || !rec.Feedback.Liked {
		t.Fatalf("feedback not applied: %+v", rec.Feedback)
	}
	body := w.Body.String()
	for _, want := range []string{
		"feedback__btn--active",         // active styling rendered
		`hx-post="/recipe/r1/feedback"`, // re-render wires back to the endpoint
		`name="liked"`,                  // independent flag inputs present
		`name="disliked"`,
		`name="cook_again"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q:\n%s", want, body)
		}
	}
}

func TestFeedbackIsIdempotent(t *testing.T) {
	rec := &domain.Recipe{ID: "r1"}
	h := newFeedbackHandler(t, stubFeedbackSetter{byID: map[string]*domain.Recipe{"r1": rec}})

	// Apply liked=true twice; both succeed and leave the same single-flag state.
	for i := 0; i < 2; i++ {
		w, req := postFeedback("r1", "liked=true")
		h.Feedback(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("call %d status = %d, want 200", i+1, w.Code)
		}
	}
	if rec.Feedback == nil || !rec.Feedback.Liked || rec.Feedback.Disliked || rec.Feedback.CookAgain {
		t.Fatalf("replay changed state: %+v", rec.Feedback)
	}
}

func TestFeedbackBlankIDIsBadRequest(t *testing.T) {
	h := newFeedbackHandler(t, stubFeedbackSetter{byID: map[string]*domain.Recipe{}})
	w, req := postFeedback("", "liked=true")
	req.SetPathValue("id", "")

	h.Feedback(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestFeedbackMissingRecipeIsBenign(t *testing.T) {
	h := newFeedbackHandler(t, stubFeedbackSetter{byID: map[string]*domain.Recipe{}})
	w, req := postFeedback("gone", "liked=true")

	h.Feedback(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (replay for a removed recipe is benign)", w.Code)
	}
}

func TestFeedbackServiceErrorIs500(t *testing.T) {
	h := newFeedbackHandler(t, stubFeedbackSetter{err: errors.New("db down")})
	w, req := postFeedback("r1", "liked=true")

	h.Feedback(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.Code)
	}
	if strings.Contains(w.Body.String(), "db down") {
		t.Errorf("internal error leaked to body:\n%s", w.Body.String())
	}
}

func TestRecipeDetailRendersFeedbackControl(t *testing.T) {
	srv := newTestRouter(t)

	// A recipe with no feedback: control present, nothing active.
	req := httptest.NewRequest(http.MethodGet, "/recipe/abc123", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	body := rec.Body.String()
	for _, want := range []string{
		"How was it?",      // localized heading (EN default)
		`class="feedback"`, // shared fragment rendered
		`hx-post="/recipe/abc123/feedback"`,
		"Cook again",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("detail body missing %q:\n%s", want, body)
		}
	}

	// A recipe that is already liked: active state rendered.
	req = httptest.NewRequest(http.MethodGet, "/recipe/liked1", nil)
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if !strings.Contains(rec.Body.String(), "feedback__btn--active") {
		t.Errorf("liked recipe should render an active feedback button:\n%s", rec.Body.String())
	}
}
