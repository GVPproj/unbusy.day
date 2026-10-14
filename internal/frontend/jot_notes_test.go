package frontend

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GVPproj/unbusy.day/internal/jot"
)

func TestPageSnapshotsAllThreeNotes(t *testing.T) {
	svc := newFakeJot()
	for _, note := range []string{"daily", "weekly", "quarterly"} {
		svc.pads[testOwner+"/"+note] = jot.Pad{Text: "saved-" + note, Version: 3}
		svc.pads["other-owner/"+note] = jot.Pad{Text: "private-" + note, Version: 9}
	}
	rec := httptest.NewRecorder()
	PageHandler(&fakeService{}, svc).ServeHTTP(rec, authedRequest(http.MethodGet, "/", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	body := rec.Body.String()
	for _, note := range []string{"daily", "weekly", "quarterly"} {
		if !strings.Contains(body, `"saved-`+note+`"`) || !strings.Contains(body, `data-note="`+note+`"`) {
			t.Errorf("page missing %s snapshot", note)
		}
	}
	if strings.Contains(body, "private-") {
		t.Fatal("page leaked another owner's notes")
	}
	if strings.Count(body, `data-jot-version="3"`) != 3 {
		t.Fatal("page missing note versions")
	}
}

func TestJotHandlerKeepsNoteAndOwnerIdentity(t *testing.T) {
	for _, note := range []string{"daily", "weekly", "quarterly"} {
		svc := newFakeJot()
		for _, owner := range []string{testOwner, "other-owner"} {
			for _, key := range []string{"daily", "weekly", "quarterly"} {
				svc.pads[owner+"/"+key] = jot.Pad{Text: owner + key, Version: 4}
			}
		}
		rec := httptest.NewRecorder()
		req := authedRequest(http.MethodPost, "/jot?note="+note, `{"_jot":"changed","_jotVersion":4,"note":"daily","owner":"other-owner"}`)
		JotHandler(svc).ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: status %d", note, rec.Code)
		}
		version, text := decodeJotResponse(t, rec)
		if version != 5 || text != nil || svc.gotBase != 4 {
			t.Fatalf("%s: response %d/%v, base %d", note, version, text, svc.gotBase)
		}
		for _, other := range []string{"daily", "weekly", "quarterly"} {
			if other != note {
				got, _ := svc.Get(req.Context(), testOwner, other)
				if got.Text != testOwner+other || got.Version != 4 {
					t.Fatalf("sibling note modified: %+v", got)
				}
			}
			got, _ := svc.Get(req.Context(), "other-owner", other)
			if got.Text != "other-owner"+other || got.Version != 4 {
				t.Fatalf("other owner modified: %+v", got)
			}
		}
		got, _ := svc.Get(req.Context(), testOwner, note)
		if got.Text != "changed" || got.Version != 5 {
			t.Fatalf("wrong note modified: %+v", got)
		}
	}
}
