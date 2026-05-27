package llm

import (
	"context"
	"errors"
	"testing"
)

// fakeClient returns scripted responses in order and counts calls, so Generate
// can be exercised without a real provider.
type fakeClient struct {
	responses []response
	calls     int
}

type response struct {
	comp Completion
	err  error
}

func (f *fakeClient) Complete(_ context.Context, _ Request) (Completion, error) {
	if f.calls >= len(f.responses) {
		return Completion{}, errors.New("fakeClient: no scripted response")
	}
	r := f.responses[f.calls]
	f.calls++
	return r.comp, r.err
}

type payload struct {
	Category string `json:"category"`
}

func TestGenerateSuccess(t *testing.T) {
	c := &fakeClient{responses: []response{
		{comp: Completion{Text: `{"category":"produce"}`}},
	}}

	got, err := Generate[payload](context.Background(), c, Request{})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if got.Category != "produce" {
		t.Fatalf("category = %q, want produce", got.Category)
	}
	if c.calls != 1 {
		t.Fatalf("calls = %d, want 1", c.calls)
	}
}

func TestGenerateRepairsInvalidJSON(t *testing.T) {
	c := &fakeClient{responses: []response{
		{comp: Completion{Text: "Sure! Here you go: {nope}"}},
		{comp: Completion{Text: `{"category":"dairy"}`}},
	}}

	got, err := Generate[payload](context.Background(), c, Request{Schema: `{"category":"string"}`})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if got.Category != "dairy" {
		t.Fatalf("category = %q, want dairy", got.Category)
	}
	if c.calls != 2 {
		t.Fatalf("calls = %d, want 2 (one repair)", c.calls)
	}
}

func TestGenerateInvalidJSONTwiceFails(t *testing.T) {
	c := &fakeClient{responses: []response{
		{comp: Completion{Text: "not json"}},
		{comp: Completion{Text: "still not json"}},
	}}

	_, err := Generate[payload](context.Background(), c, Request{})
	if !errors.Is(err, ErrInvalidJSON) {
		t.Fatalf("err = %v, want ErrInvalidJSON", err)
	}
	if c.calls != 2 {
		t.Fatalf("calls = %d, want 2 (one repair, then give up)", c.calls)
	}
}

func TestGenerateTransportErrorNoRepair(t *testing.T) {
	c := &fakeClient{responses: []response{
		{err: ErrTransient},
	}}

	_, err := Generate[payload](context.Background(), c, Request{})
	if !errors.Is(err, ErrTransient) {
		t.Fatalf("err = %v, want ErrTransient", err)
	}
	if c.calls != 1 {
		t.Fatalf("calls = %d, want 1 (no repair on transport error)", c.calls)
	}
}
