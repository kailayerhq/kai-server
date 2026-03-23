package api

import (
	"encoding/json"
	"log"
	"net/http"

	"kailab-control/internal/db"
)

// SubmitSignup handles public early access signup submissions.
func (h *Handler) SubmitSignup(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name    string `json:"name"`
		Email   string `json:"email"`
		Company string `json:"company"`
		RepoURL string `json:"repo_url"`
		AIUsage string `json:"ai_usage"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", err)
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name required", nil)
		return
	}
	if req.Email == "" {
		writeError(w, http.StatusBadRequest, "email required", nil)
		return
	}

	signup, err := h.db.CreateSignup(req.Name, req.Email, req.Company, req.RepoURL, req.AIUsage)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create signup", err)
		return
	}

	writeJSON(w, http.StatusCreated, signup)
}

// ListSignups returns all signups (admin only).
func (h *Handler) ListSignups(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	if user == nil || user.Email != h.cfg.AdminEmail {
		writeError(w, http.StatusForbidden, "admin access required", nil)
		return
	}
	status := r.URL.Query().Get("status")
	signups, err := h.db.ListSignups(status)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list signups", err)
		return
	}
	if signups == nil {
		signups = []*db.Signup{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"signups": signups})
}

// UpdateSignup updates a signup's status and notes (admin only).
func (h *Handler) UpdateSignup(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	if user == nil || user.Email != h.cfg.AdminEmail {
		writeError(w, http.StatusForbidden, "admin access required", nil)
		return
	}
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "id required", nil)
		return
	}

	var req struct {
		Status string `json:"status"`
		Notes  string `json:"notes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", err)
		return
	}

	valid := map[string]bool{"pending_review": true, "approved": true, "rejected": true, "contacted": true}
	if !valid[req.Status] {
		writeError(w, http.StatusBadRequest, "invalid status: must be pending_review, approved, rejected, or contacted", nil)
		return
	}

	// Fetch signup before updating so we can send the approval email
	signup, err := h.db.GetSignupByID(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "signup not found", err)
		return
	}

	if err := h.db.UpdateSignupStatus(id, req.Status, req.Notes); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update signup", err)
		return
	}

	// Send approval email when status changes to approved
	if req.Status == "approved" && signup.Status != "approved" && h.email != nil {
		loginURL := h.cfg.BaseURL
		if err := h.email.SendEarlyAccessApproved(signup.Email, signup.Name, loginURL); err != nil {
			log.Printf("Failed to send approval email to %s: %v", signup.Email, err)
		}
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

// ListCIRequests returns users who have requested CI access (admin only).
func (h *Handler) ListCIRequests(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	if user == nil || user.Email != h.cfg.AdminEmail {
		writeError(w, http.StatusForbidden, "admin access required", nil)
		return
	}

	users, err := h.db.ListCIRequests()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list CI requests", err)
		return
	}

	type userBrief struct {
		ID          string `json:"id"`
		Email       string `json:"email"`
		Name        string `json:"name,omitempty"`
		CIAccess    bool   `json:"ci_access"`
		CIRequested bool   `json:"ci_requested"`
	}
	var result []userBrief
	for _, u := range users {
		result = append(result, userBrief{
			ID: u.ID, Email: u.Email, Name: u.Name,
			CIAccess: u.CIAccess, CIRequested: u.CIRequested,
		})
	}
	if result == nil {
		result = []userBrief{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"users": result})
}

// SetUserCIAccess toggles CI access for a user (admin only).
// Accepts either user_id or email to identify the user.
func (h *Handler) SetUserCIAccess(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	if user == nil || user.Email != h.cfg.AdminEmail {
		writeError(w, http.StatusForbidden, "admin access required", nil)
		return
	}

	var req struct {
		UserID   string `json:"user_id"`
		Email    string `json:"email"`
		CIAccess bool   `json:"ci_access"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", err)
		return
	}

	// Resolve user ID from email if needed
	targetID := req.UserID
	if targetID == "" && req.Email != "" {
		u, err := h.db.GetUserByEmail(req.Email)
		if err != nil {
			writeError(w, http.StatusNotFound, "user not found", nil)
			return
		}
		targetID = u.ID
	}
	if targetID == "" {
		writeError(w, http.StatusBadRequest, "user_id or email required", nil)
		return
	}

	if err := h.db.UpdateUserCIAccess(targetID, req.CIAccess); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update CI access", err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}
