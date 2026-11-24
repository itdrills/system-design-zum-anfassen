package main

import (
	"context"
	"log"
	"strings"

	"github.com/nats-io/nats.go"
)

// KURZ_KLICKS legt fest, wie kurz Weiterleitungen zählt: leer = gar nicht,
// "sync" = jede Weiterleitung schreibt sofort in PostgreSQL,
// "nats://…" = jede Weiterleitung veröffentlicht eine Nachricht, ein Worker zählt.
var (
	klicksModus = env("KURZ_KLICKS", "")
	nc          *nats.Conn
)

func initKlicks() {
	if !strings.HasPrefix(klicksModus, "nats://") {
		return
	}
	var err error
	nc, err = nats.Connect(klicksModus, nats.Name(instanz),
		nats.RetryOnFailedConnect(true), // kurz startet auch, wenn NATS gerade fehlt
		nats.MaxReconnects(-1))          // und gibt das Wiederverbinden nie auf
	if err != nil {
		log.Fatalf("nats: %v", err)
	}
	log.Printf("%s: Klicks über NATS %s", instanz, nc.ConnectedUrlRedacted())
}

// zaehle wird bei jeder Weiterleitung aufgerufen, bevor die Antwort rausgeht.
func zaehle(ctx context.Context, code string) {
	var err error
	switch {
	case klicksModus == "sync":
		_, err = db.Exec(ctx, `INSERT INTO klicks (code, anzahl) VALUES ($1, 1)
			ON CONFLICT (code) DO UPDATE SET anzahl = klicks.anzahl + 1`, code)
	case nc != nil:
		err = nc.Publish("klicks", []byte(code)) // wartet auf niemanden
	}
	if err != nil {
		log.Printf("zaehle: %v", err) // ein verlorener Klick, die Weiterleitung klappt trotzdem
	}
}
