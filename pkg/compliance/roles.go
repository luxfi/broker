package compliance

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
)

// rolesHandler holds RBAC HTTP handler state.
type rolesHandler struct {
	store ComplianceStore
}

func (h *rolesHandler) handleListRoles(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.store.ListRoles())
}

func (h *rolesHandler) handleCreateRole(w http.ResponseWriter, r *http.Request) {
	var role Role
	if err := json.NewDecoder(r.Body).Decode(&role); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if role.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if err := h.store.SaveRole(&role); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, &role)
}

func (h *rolesHandler) handleGetRole(w http.ResponseWriter, r *http.Request) {
	role, err := h.store.GetRole(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, role)
}

func (h *rolesHandler) handleUpdateRole(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	existing, err := h.store.GetRole(id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	var patch struct {
		Name        *string      `json:"name,omitempty"`
		Description *string      `json:"description,omitempty"`
		Permissions []Permission `json:"permissions,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if patch.Name != nil {
		existing.Name = *patch.Name
	}
	if patch.Description != nil {
		existing.Description = *patch.Description
	}
	if patch.Permissions != nil {
		existing.Permissions = patch.Permissions
	}
	if err := h.store.SaveRole(existing); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, existing)
}

func (h *rolesHandler) handleDeleteRole(w http.ResponseWriter, r *http.Request) {
	if err := h.store.DeleteRole(chi.URLParam(r, "id")); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// handleListModules returns the static list of compliance modules and their
// available actions, used to build a permission matrix UI.
func (h *rolesHandler) handleListModules(w http.ResponseWriter, r *http.Request) {
	modules := []Module{
		{Name: "kyc", Description: "KYC identity verification", Actions: []string{"read", "write", "admin"}},
		{Name: "funds", Description: "Fund management", Actions: []string{"read", "write", "delete", "admin"}},
		{Name: "esign", Description: "Electronic signatures", Actions: []string{"read", "write", "admin"}},
		{Name: "pipelines", Description: "Onboarding pipelines", Actions: []string{"read", "write", "delete", "admin"}},
		{Name: "sessions", Description: "Investor onboarding sessions", Actions: []string{"read", "write", "admin"}},
		{Name: "roles", Description: "Role-based access control", Actions: []string{"read", "write", "delete", "admin"}},
	}
	writeJSON(w, http.StatusOK, modules)
}
