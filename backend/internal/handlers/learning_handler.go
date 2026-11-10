package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog/log"

	"github.com/halal-equity/backend/internal/database"
	"github.com/halal-equity/backend/pkg/response"
)

// LearningHandler serves the governed learning loop: strategy versions, learner runs, lessons, and the owner's rollback.
type LearningHandler struct{ db *database.DB }

func NewLearningHandler(db *database.DB) *LearningHandler { return &LearningHandler{db: db} }

func (h *LearningHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/summary", h.Summary)
	r.Post("/versions/{id}/rollback", h.Rollback)
	return r
}

// Summary returns the versions, the latest run with its gates and P&L audit, lesson aggregates and recent lessons.
func (h *LearningHandler) Summary(w http.ResponseWriter, r *http.Request) {
	s, err := h.db.GetLearningSummary(r.Context())
	if err != nil {
		log.Error().Err(err).Msg("learning summary failed")
		response.Error(w, http.StatusInternalServerError, "failed to load the learning summary")
		return
	}
	response.JSON(w, http.StatusOK, s)
}

// Rollback puts the frozen control back in charge of paper trading. Only the active version can be rolled back.
func (h *LearningHandler) Rollback(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		response.ValidationError(w, "invalid version id")
		return
	}
	var body struct {
		Reason string `json:"reason"`
	}
	_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&body)
	if len(body.Reason) > 500 {
		body.Reason = body.Reason[:500]
	}
	switch err := h.db.RollbackActiveVersion(r.Context(), id, body.Reason); {
	case errors.Is(err, database.ErrNotActiveVersion):
		response.Error(w, http.StatusConflict, err.Error())
	case errors.Is(err, sql.ErrNoRows):
		response.Error(w, http.StatusNotFound, "no such strategy version")
	case err != nil:
		log.Error().Err(err).Msg("rollback failed")
		response.Error(w, http.StatusInternalServerError, "rollback failed")
	default:
		response.JSON(w, http.StatusOK, map[string]any{"rolled_back": id})
	}
}
