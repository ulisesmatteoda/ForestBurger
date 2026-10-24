package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"

	sqlc "forestburger/db/sqlc"
)

// API agrupa los handlers y les da acceso a las queries generadas por sqlc.
type API struct {
	q *sqlc.Queries
}

// NewAPI arma la API con las queries (q queda privado al paquete).
func NewAPI(q *sqlc.Queries) *API {
	return &API{q: q}
}

// ---------- Helpers compartidos ----------

// toNullString convierte un *string del JSON (nil si no vino) al sql.NullString que espera sqlc.
func toNullString(s *string) sql.NullString {
	if s == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: *s, Valid: true}
}

// nullToPtr hace lo inverso: de sql.NullString a *string, para que en el JSON salga null.
func nullToPtr(ns sql.NullString) *string {
	if !ns.Valid {
		return nil
	}
	return &ns.String
}

// floatToStr pasa un precio a string con 2 decimales, porque sqlc mapea DECIMAL a string.
func floatToStr(f float64) string {
	return strconv.FormatFloat(f, 'f', 2, 64)
}

// ----------------helpers----------------

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func pathID(r *http.Request) (int32, error) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 32)
	return int32(id), err
}

// floatPtrToNull pasa un precio opcional del JSON (nil si no vino) al DECIMAL nullable de sqlc.
func floatPtrToNull(f *float64) sql.NullString {
	if f == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: floatToStr(*f), Valid: true}
}

// nullToFloatPtr hace lo inverso: de DECIMAL nullable a *float64, para que en el JSON salga null.
func nullToFloatPtr(ns sql.NullString) *float64 {
	if !ns.Valid {
		return nil
	}
	f, _ := strconv.ParseFloat(ns.String, 64)
	return &f
}