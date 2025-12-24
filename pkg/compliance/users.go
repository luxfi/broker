package compliance

import (
	"encoding/json"
	"net/http"
	"time"
)

// usersHandler holds user management HTTP handler state.
type usersHandler struct {
	store ComplianceStore
}

func (h *usersHandler) handleListUsers(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.store.ListUsers())
}

func (h *usersHandler) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	var u User
	if err := json.NewDecoder(r.Body).Decode(&u); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if u.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if u.Email == "" {
		writeError(w, http.StatusBadRequest, "email is required")
		return
	}
	if u.Role == "" {
		u.Role = "agent"
	}
	if u.Status == "" {
		u.Status = "active"
	}
	if u.LastLogin.IsZero() {
		u.LastLogin = time.Now().UTC()
	}
	if err := h.store.SaveUser(&u); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, &u)
}
