package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/url"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ausfuehrer: ein Pool oder eine laufende Transaktion, beide können Exec und QueryRow.
type ausfuehrer interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type txSchluessel struct{}

// verbindung: Hält die Anfrage schon eine Transaktion auf db (Idempotency-Key),
// läuft alles auf db über sie. Sonst nimmt sie eine freie Verbindung aus dem Pool.
func verbindung(ctx context.Context, p *pgxpool.Pool) ausfuehrer {
	if tx, ok := ctx.Value(txSchluessel{}).(pgx.Tx); ok && p == db {
		return tx
	}
	return p
}

func anlegen(w http.ResponseWriter, r *http.Request) {
	var in struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, "JSON erwartet", http.StatusBadRequest)
		return
	}
	u, err := url.Parse(in.URL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		http.Error(w, "ungültige URL", http.StatusBadRequest)
		return
	}
	ctx := r.Context()
	// Gibt es die Adresse schon, liefert kurz den vorhandenen Code statt eines neuen
	vorhanden, err := sucheURL(ctx, in.URL)
	if err == nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"code": vorhanden, "url": in.URL})
		return
	}
	if !errors.Is(err, pgx.ErrNoRows) { // unklar, ob es sie gibt: lieber nichts anlegen
		log.Printf("anlegen: %v", err)
		http.Error(w, "Datenbankfehler", http.StatusServiceUnavailable)
		return
	}
	time.Sleep(verzoegerung)
	for versuch := 0; versuch < 3; versuch++ {
		code := neuerCode()
		// ON CONFLICT statt Fehler 23505: Ein Fehler bräche die Transaktion ab.
		tag, err := verbindung(ctx, shards[shardFuer(code)]).Exec(ctx,
			`INSERT INTO links (code, url) VALUES ($1, $2)
			ON CONFLICT DO NOTHING`, code, in.URL)
		if err != nil {
			log.Printf("anlegen: %v", err)
			http.Error(w, "Datenbankfehler", http.StatusServiceUnavailable)
			return
		}
		if tag.RowsAffected() == 0 {
			// Code vergeben oder die Adresse kam gerade von einer anderen Anfrage
			if vorhanden, err := sucheURL(ctx, in.URL); err == nil {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(
					map[string]string{"code": vorhanden, "url": in.URL})
				return
			}
			continue
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{"code": code, "url": in.URL})
		return
	}
	http.Error(w, "kein freier Code", http.StatusInternalServerError)
}
