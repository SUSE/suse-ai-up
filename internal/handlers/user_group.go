// ListGroupAdapters lists adapters assigned to a group
// ListGroupAdapters handles GET /api/v1/groups/{id}/adapters
// @Summary List group adapters
// @Description List all adapters assigned to a specific group
// @Tags groups
// @Produce json
// @Param id path string true "Group ID"
// @Success 200 {array} models.AdapterGroupAssignment
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/groups/{id}/adapters [get]
func (h *UserGroupHandler) ListGroupAdapters(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract group ID from URL path
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/groups/")
	parts := strings.Split(path, "/")
	if len(parts) < 2 || parts[1] != "adapters" {
		http.NotFound(w, r)
		return
	}
	groupID := parts[0]

	userID := r.Header.Get("X-User-ID")
	if userID == "" {
		userID = "default-user"
	}

	if h.adapterService == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "Adapter service not available"})
		return
	}

	assignments, err := h.adapterService.ListGroupAdapters(r.Context(), userID, groupID, h.userGroupService)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(err.Error(), "denied") {
			w.WriteHeader(http.StatusForbidden)
			json.NewEncoder(w).Encode(ErrorResponse{Error: "Access denied"})
		} else {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(ErrorResponse{Error: "Failed to list group adapters: " + err.Error()})
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(assignments)
}

// ListGroupAdapters lists adapters assigned to a group
// ListGroupAdapters handles GET /api/v1/groups/{id}/adapters
// @Summary List group adapters
// @Description List all adapters assigned to a specific group
// @Tags groups
// @Produce json
// @Param id path string true "Group ID"
// @Success 200 {array} models.AdapterGroupAssignment
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/groups/{id}/adapters [get]
func (h *UserGroupHandler) ListGroupAdapters(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract group ID from URL path
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/groups/")
	parts := strings.Split(path, "/")
	if len(parts) < 2 || parts[1] != "adapters" {
		http.NotFound(w, r)
		return
	}
	groupID := parts[0]

	userID := r.Header.Get("X-User-ID")
	if userID == "" {
		userID = "default-user"
	}

	if h.adapterService == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "Adapter service not available"})
		return
	}

	assignments, err := h.adapterService.ListGroupAdapters(r.Context(), userID, groupID, h.userGroupService)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(err.Error(), "denied") {
			w.WriteHeader(http.StatusForbidden)
			json.NewEncoder(w).Encode(ErrorResponse{Error: "Access denied"})
		} else {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(ErrorResponse{Error: "Failed to list group adapters: " + err.Error()})
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(assignments)
}