package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"io"
	"log"
	"net/http"
)

// aufnahme hält Status und Body einer Antwort fest, statt sie gleich zu senden.
type aufnahme struct {
	header http.Header
	status int
	body   bytes.Buffer
}

func (a *aufnahme) Header() http.Header         { return a.header }
func (a *aufnahme) Write(b []byte) (int, error) { return a.body.Write(b) }
func (a *aufnahme) WriteHeader(status int)      { a.status = status }

// idempotent: Kommt ein Idempotency-Key, den es schon gibt, antwortet kurz
// mit der gespeicherten Antwort, statt die Anfrage noch einmal auszuführen.
func idempotent(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		schluessel := r.Header.Get("Idempotency-Key")
		if schluessel == "" {
			next(w, r)
			return
		}
		if len(schluessel) > 200 {
			http.Error(w, "Idempotency-Key zu lang", http.StatusBadRequest)
			return
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			http.Error(w, "Body nicht lesbar", http.StatusBadRequest)
			return
		}
		hash := sha256.Sum256(body)
		ctx := r.Context()
		tx, err := db.Begin(ctx)
		if err != nil {
			http.Error(w, "Datenbankfehler", http.StatusServiceUnavailable)
			return
		}
		defer tx.Rollback(ctx) // ohne Commit gibt es den Schlüssel nicht
		// Läuft eine Anfrage mit demselben Schlüssel, wartet das INSERT auf ihr Ende.
		tag, err := tx.Exec(ctx, `INSERT INTO idempotenz (schluessel, anfrage)
			VALUES ($1, $2) ON CONFLICT (schluessel) DO NOTHING`, schluessel, hash[:])
		if err != nil {
			log.Printf("idempotenz: %v", err)
			http.Error(w, "Datenbankfehler", http.StatusServiceUnavailable)
			return
		}
		if tag.RowsAffected() == 0 { // Schlüssel schon benutzt
			var anfrage, antwort []byte
			var status int
			err := tx.QueryRow(ctx, `SELECT anfrage, status, antwort FROM idempotenz
				WHERE schluessel = $1`, schluessel).Scan(&anfrage, &status, &antwort)
			if err != nil {
				http.Error(w, "Datenbankfehler", http.StatusServiceUnavailable)
				return
			}
			if !bytes.Equal(anfrage, hash[:]) {
				http.Error(w, "Idempotency-Key gehört zu einer anderen Anfrage",
					http.StatusUnprocessableEntity)
				return
			}
			if status < 300 {
				w.Header().Set("Content-Type", "application/json")
			}
			w.Header().Set("Idempotent-Replayed", "true")
			w.WriteHeader(status)
			w.Write(antwort)
			return
		}
		a := &aufnahme{header: w.Header(), status: http.StatusOK}
		r.Body = io.NopCloser(bytes.NewReader(body))
		next(a, r.WithContext(context.WithValue(ctx, txSchluessel{}, tx)))
		// Serverfehler nicht speichern: Ein neuer Versuch soll es neu probieren.
		if a.status < 500 {
			_, err = tx.Exec(ctx, `UPDATE idempotenz SET status = $2, antwort = $3
				WHERE schluessel = $1`, schluessel, a.status, a.body.Bytes())
			if err == nil {
				err = tx.Commit(ctx)
			}
			if err != nil {
				log.Printf("idempotenz: %v", err)
			}
		}
		w.WriteHeader(a.status)
		w.Write(a.body.Bytes())
	}
}
