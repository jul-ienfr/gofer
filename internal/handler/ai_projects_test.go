package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAIProjectsAPICreatesUpdatesAndRoutes(t *testing.T) {
	h, db := newAIHandler(t)
	if _, err := db.Write().Exec(`INSERT INTO accounts (id, user_id, email_address, provider) VALUES ('acc', 'owner', 'owner@example.com', 'imap')`); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	h.handleAIProjects(w, aiRequest(http.MethodPost, "/api/ai/projects", "owner", `{
		"name":"Employment","slug":"employment","allow_external_routing":true,"data_classification":"internal"
	}`))
	if w.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body %s", w.Code, w.Body.String())
	}
	var created struct {
		ID                 string `json:"ID"`
		AllowLLMDraft      *bool  `json:"AllowLLMDraft"`
		DataClassification string `json:"DataClassification"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.ID == "" || created.DataClassification != "internal" || created.AllowLLMDraft == nil || !*created.AllowLLMDraft {
		t.Fatalf("created = %+v", created)
	}
	allowDraft := false
	w = httptest.NewRecorder()
	r := aiRequest(http.MethodPatch, "/api/ai/projects/"+created.ID, "owner", `{"allow_llm_draft":false,"status":"paused"}`)
	r.SetPathValue("id", created.ID)
	h.handleAIProject(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("patch status = %d, body %s", w.Code, w.Body.String())
	}
	var patched struct {
		AllowLLMDraft        *bool  `json:"AllowLLMDraft"`
		AllowExternalRouting bool   `json:"AllowExternalRouting"`
		Status               string `json:"Status"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &patched); err != nil {
		t.Fatal(err)
	}
	if patched.Status != "paused" || patched.AllowLLMDraft == nil || *patched.AllowLLMDraft != allowDraft {
		t.Fatalf("patched = %+v", patched)
	}
	w = httptest.NewRecorder()
	r = aiRequest(http.MethodPatch, "/api/ai/projects/"+created.ID, "owner", `{"data_classification":"restricted"}`)
	r.SetPathValue("id", created.ID)
	h.handleAIProject(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("restricted patch status = %d, body %s", w.Code, w.Body.String())
	}
	if err := json.Unmarshal(w.Body.Bytes(), &patched); err != nil {
		t.Fatal(err)
	}
	if patched.AllowExternalRouting {
		t.Fatalf("restricted project kept external routing: %+v", patched)
	}
	w = httptest.NewRecorder()
	h.handleAIProjects(w, aiRequest(http.MethodPost, "/api/ai/projects", "owner", `{"name":"Bad","slug":"bad","data_classification":"secret"}`))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("invalid classification status = %d, body %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	h.handleAIProjects(w, aiRequest(http.MethodPost, "/api/ai/projects", "owner", `{"name":"Confidential","slug":"confidential","allow_external_routing":true,"data_classification":"confidential"}`))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("confidential routing status = %d, body %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	r = aiRequest(http.MethodPost, "/api/ai/projects/"+created.ID+"/routes", "owner", `{
		"name":"Recruiters","enabled":true,"priority":10,
		"match_json":"{\"senders\":[\"alice@example.com\"]}",
		"actions_json":"[{\"kind\":\"notify\"}]"
	}`)
	r.SetPathValue("id", created.ID)
	h.handleAIProjectRoutes(w, r)
	if w.Code != http.StatusCreated {
		t.Fatalf("route status = %d, body %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	r = aiRequest(http.MethodPost, "/api/ai/projects/"+created.ID+"/routes", "owner", `{
		"name":"Bad","enabled":true,"match_json":"{}","actions_json":"[{\"kind\":\"delete_all\"}]"
	}`)
	r.SetPathValue("id", created.ID)
	h.handleAIProjectRoutes(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("unsafe route status = %d, body %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	r = aiRequest(http.MethodGet, "/api/ai/projects", "owner", ``)
	h.handleAIProjects(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("list status = %d", w.Code)
	}
}

func TestAILegitimacyAPIUpdatesOnlyTheOwner(t *testing.T) {
	h, db := newAIHandler(t)
	ctx := context.Background()
	if _, err := db.Write().Exec(`INSERT INTO accounts (id, user_id, email_address, provider) VALUES ('acc', 'owner', 'owner@example.com', 'imap')`); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	r := aiRequest(http.MethodPut, "/api/ai/legitimacy/alice@example.com", "owner", `{"vip":true,"allowlisted":true}`)
	r.SetPathValue("email", "alice@example.com")
	h.handleAILegitimacyProfile(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", w.Code, w.Body.String())
	}
	profile, err := db.GetAILegitimacyProfile(ctx, "acc", "alice@example.com")
	if err != nil || !profile.VIP || !profile.Allowlisted {
		t.Fatalf("profile = %+v, %v", profile, err)
	}
	w = httptest.NewRecorder()
	r = aiRequest(http.MethodPut, "/api/ai/legitimacy/alice@example.com", "owner", `{"blocked":true}`)
	r.SetPathValue("email", "alice@example.com")
	h.handleAILegitimacyProfile(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("block status = %d, body %s", w.Code, w.Body.String())
	}
	profile, err = db.GetAILegitimacyProfile(ctx, "acc", "alice@example.com")
	if err != nil || !profile.Blocked || profile.Allowlisted {
		t.Fatalf("blocked profile = %+v, %v", profile, err)
	}
	w = httptest.NewRecorder()
	r = aiRequest(http.MethodGet, "/api/ai/legitimacy", "owner", ``)
	h.handleAILegitimacyProfiles(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("list status = %d, body %s", w.Code, w.Body.String())
	}
}
