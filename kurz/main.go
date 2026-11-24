// kurz: ein kleiner URL-Shortener. Version 1: eine Instanz, eine Datenbank.
package main

import (
	"context"
	"crypto/rand"
	"errors"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const alphabet = "abcdefghijkmnopqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789"

var (
	db      *pgxpool.Pool
	lesen   *pgxpool.Pool // Lese-Replikat, ohne KURZ_DB_LESEN dieselbe Datenbank wie db
	instanz = env("KURZ_INSTANZ", "kurz")
	// Codelänge, Standard 7. Nur zum Ausprobieren kleiner Code-Räume
	codeLaenge, _ = strconv.Atoi(env("KURZ_CODELAENGE", "7"))
	// Nur für Tag 11: Pause vor dem INSERT
	verzoegerung, _ = time.ParseDuration(env("KURZ_VERZOEGERUNG", "0s"))
	// Höchstdauer einer Anfrage: danach bricht kurz Datenbank- und Cache-Zugriffe ab
	anfrageTimeout, _ = time.ParseDuration(env("KURZ_TIMEOUT", "2s"))
)

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// neuerCode erzeugt 7 zufällige Zeichen (57^7 ≈ 1,9 Billionen Möglichkeiten).
func neuerCode() string {
	b := make([]byte, codeLaenge)
	rand.Read(b)
	for i := range b {
		b[i] = alphabet[int(b[i])%len(alphabet)]
	}
	return string(b)
}

func weiterleiten(w http.ResponseWriter, r *http.Request) {
	ziel, err := finde(r.Context(), r.PathValue("code"))
	if errors.Is(err, pgx.ErrNoRows) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		log.Printf("weiterleiten: %v", err)
		http.Error(w, "Datenbankfehler", http.StatusServiceUnavailable)
		return
	}
	zaehle(r.Context(), r.PathValue("code"))
	http.Redirect(w, r, ziel, http.StatusFound)
}

func bereit(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), time.Second)
	defer cancel()
	if !cacheBereit(ctx) && db.Ping(ctx) != nil {
		http.Error(w, "Datenbank nicht erreichbar", http.StatusServiceUnavailable)
		return
	}
	w.Write([]byte("bereit\n"))
}

func main() {
	var err error
	dsn := env("KURZ_DB", "postgres://kurz:lab-kurz-123@s3:5432/kurz")
	db, err = pgxpool.New(context.Background(), dsn)
	if err != nil {
		log.Fatal(err)
	}
	lesen = db
	if dsnLesen := env("KURZ_DB_LESEN", ""); dsnLesen != "" {
		if lesen, err = pgxpool.New(context.Background(), dsnLesen); err != nil {
			log.Fatal(err)
		}
	}
	if _, err := db.Exec(context.Background(), `CREATE TABLE IF NOT EXISTS links (
		code text PRIMARY KEY,
		url text NOT NULL,
		angelegt timestamptz NOT NULL DEFAULT now())`); err != nil {
		log.Fatal(err)
	}
	initShards()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/links", gesichert(schreiben, idempotent(anlegen)))
	mux.HandleFunc("GET /{code}", weiterleiten)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok\n"))
	})
	mux.HandleFunc("GET /readyz", bereit)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Kurz-Instanz", instanz)
		ctx, cancel := context.WithTimeout(r.Context(), anfrageTimeout)
		defer cancel()
		mux.ServeHTTP(w, r.WithContext(ctx))
	})
	initCache()
	initKlicks()
	serve(handler)
}
